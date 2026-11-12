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
    ctx.configure(underlying="NIFTY")
    expiries = ctx.get_expiries(start="2026-09-01", end="2026-10-31")
    if expiries != ["2026-09-29", "2026-10-27"]:
        fail("incorrect expiries")
    for expiry in reversed(expiries):
        print("start " + expiry)
        ctx.state["initialization_value"] = True
        ctx.simulate(expiry)
        if "visited" not in ctx.state:
            fail("simulate returned before callbacks")
        print("done " + expiry)

def on_candle(ctx):
    if "initialization_value" in ctx.state:
        fail("state leaked into simulation")
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
	// that callbacks finished synchronously and state was reset for both runs.
	want := "initialize|start 2026-10-27|done 2026-10-27|start 2026-09-29|done 2026-09-29"
	if strings.Join(messages, "|") != want {
		t.Fatalf("messages: %v", messages)
	}
	if len(result.Executions) != 2 || result.Underlying != "NIFTY" {
		t.Fatalf("result: %+v", result)
	}
}

func TestSimulationAPIErrors(t *testing.T) {
	for _, tt := range []struct{ name, body, callback, want string }{
		{"invalid start", `ctx.get_expiries(start="bad", end="2026-10-31")`, "pass", "start:"},
		{"invalid end", `ctx.get_expiries(start="2026-09-01", end="bad")`, "pass", "end:"},
		{"reversed range", `ctx.get_expiries(start="2026-10-31", end="2026-09-01")`, "pass", "must not be after"},
		{"invalid expiry", `ctx.simulate("bad")`, "pass", "expiry:"},
		{"callback failure", `ctx.simulate("2026-09-29")`, `fail("callback failed")`, "callback failed"},
		{"nested simulation", `ctx.simulate("2026-09-29")`, `ctx.simulate("2026-10-27")`, "simulate requires configure during initialize"},
		{"late discovery", `ctx.simulate("2026-09-29")`, `ctx.get_expiries(start="2026-09-01", end="2026-10-31")`, "get_expiries requires configure during initialize"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			source := "def initialize(ctx):\n    ctx.configure(underlying='NIFTY')\n    " + tt.body + "\ndef on_candle(ctx):\n    " + tt.callback + "\n"
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
