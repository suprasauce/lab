package scripting

import (
	"context"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"optionsbacktest/engine"
)

func TestScriptControlsSynchronousExpiryOrder(t *testing.T) {
	f := newFixture(t)
	script, err := LoadSource("simulation.star", `
def initialize(ctx):
    print("initialize")
    ctx.configure()
    expiries = ctx.get_expiries(underlying="NIFTY", start="2026-09-01", end="2026-10-31")
    if expiries != [{"date": "2026-09-29", "underlying": "NIFTY"}, {"date": "2026-10-27", "underlying": "NIFTY"}]:
        fail("incorrect expiries")
    for expiry in reversed(expiries):
        print("start " + expiry["date"])
        ctx.clear_state()
        ctx.state["expiry"] = expiry
        ctx.simulate(expiry)
        if "visited" not in ctx.state:
            fail("simulate returned before callbacks")
        print("done " + expiry["date"])

def on_candle(ctx):
    if "expiry" not in ctx.state:
        fail("state was cleared automatically")
    if "visited" not in ctx.state:
        ctx.state["visited"] = True
`)
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	script.thread.Print = func(_ *starlark.Thread, message string) { messages = append(messages, message) }
	result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
	if err != nil {
		t.Fatal(err)
	}
	// Callback prints use their own thread; completion checks in initialize prove
	// that callbacks finished synchronously with script-managed state resets.
	want := "initialize|start 2026-10-27|done 2026-10-27|start 2026-09-29|done 2026-09-29"
	if strings.Join(messages, "|") != want {
		t.Fatalf("messages: %v", messages)
	}
	if len(result.Executions) != 2 {
		t.Fatalf("result: %+v", result)
	}
}

func TestSimulationAPIErrors(t *testing.T) {
	for _, tt := range []struct{ name, body, callback, want string }{
		{"invalid start", `ctx.get_expiries(underlying="NIFTY", start="bad", end="2026-10-31")`, "pass", "start:"},
		{"invalid end", `ctx.get_expiries(underlying="NIFTY", start="2026-09-01", end="bad")`, "pass", "end:"},
		{"reversed range", `ctx.get_expiries(underlying="NIFTY", start="2026-10-31", end="2026-09-01")`, "pass", "must not be after"},
		{"invalid expiry", `ctx.simulate({"date":"bad", "underlying":"NIFTY"})`, "pass", "expiry:"},
		{"callback failure", `ctx.simulate({"date":"2026-09-29", "underlying":"NIFTY"})`, `fail("callback failed")`, "callback failed"},
		{"nested simulation", `ctx.simulate({"date":"2026-09-29", "underlying":"NIFTY"})`, `ctx.simulate({"date":"2026-10-27", "underlying":"NIFTY"})`, "simulate requires configure during initialize"},
		{"late discovery", `ctx.simulate({"date":"2026-09-29", "underlying":"NIFTY"})`, `ctx.get_expiries(underlying="NIFTY", start="2026-09-01", end="2026-10-31")`, "get_expiries requires configure during initialize"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			source := "def initialize(ctx):\n    ctx.configure()\n    " + tt.body + "\ndef on_candle(ctx):\n    " + tt.callback + "\n"
			script, err := LoadSource("error.star", source)
			if err != nil {
				t.Fatal(err)
			}
			_, err = script.Run(context.Background(), engine.NewEngine(f.repo))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want %s", err, tt.want)
			}
		})
	}
}

func TestEmptyExpiryDiscovery(t *testing.T) {
	f := newFixture(t)
	f.expiries = nil
	script, err := LoadSource("empty.star", setup+"\ndef on_candle(ctx):\n    fail('unexpected callback')\n")
	if err != nil {
		t.Fatal(err)
	}
	result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Executions) != 0 {
		t.Fatal("simulated an unrequested expiry")
	}
}

func TestStatePersistsAcrossExpiriesButNotBacktests(t *testing.T) {
	script, err := LoadSource("state.star", `
def initialize(ctx):
    if len(ctx.state) != 0:
        fail("new backtest inherited script state")
    ctx.configure()
    ctx.state["expiry_count"] = 0
    for expiry in ctx.get_expiries(underlying="NIFTY", start="2026-09-01", end="2026-10-31"):
        ctx.state["expiry"] = expiry
        ctx.simulate(expiry)
    if ctx.state["expiry_count"] != 2:
        fail("state did not persist across expiries")

def on_candle(ctx):
    if ctx.dte == 45 and ctx.time == "09:15":
        ctx.state["expiry_count"] += 1
`)
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		f := newFixture(t)
		if _, err := script.Run(context.Background(), engine.NewEngine(f.repo)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClearStateClearsExistingDictionary(t *testing.T) {
	c := &strategyContext{state: starlark.NewDict(0)}
	globals := starlark.StringDict{"ctx": c}
	_, err := starlark.ExecFile(&starlark.Thread{Name: "clear-state"}, "clear.star", `
def check(ctx):
    saved = ctx.state
    ctx.state["temporary"] = {"nested": [1, 2]}
    ctx.clear_state()
    if len(ctx.state) != 0 or len(saved) != 0:
        fail("state was not cleared in place")
check(ctx)
`, globals)
	if err != nil {
		t.Fatal(err)
	}
}
