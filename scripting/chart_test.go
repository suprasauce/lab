package scripting

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"optionsbacktest/engine"
)

func TestChartEMA(t *testing.T) {
	for _, tt := range []struct{ name, expression, want string }{
		{"keyword arguments", `chart.ema(tradingsymbol="NIFTY", period=20, timeframe="5m")`, "EMA not implemented"},
		{"positional arguments", `chart.ema("NIFTY", 20, "5m")`, "EMA not implemented"},
		{"missing argument", `chart.ema("NIFTY", 20)`, "missing"},
		{"wrong symbol type", `chart.ema(1, 20, "5m")`, "want string"},
		{"empty symbol", `chart.ema(" ", 20, "5m")`, "tradingsymbol must be nonempty"},
		{"zero period", `chart.ema("NIFTY", 0, "5m")`, "period must be positive"},
		{"negative period", `chart.ema("NIFTY", -1, "5m")`, "period must be positive"},
		{"fractional period", `chart.ema("NIFTY", 2.5, "5m")`, "want int"},
		{"unsupported timeframe", `chart.ema("NIFTY", 20, "2m")`, "unsupported timeframe"},
		{"one-minute timeframe", `chart.ema("NIFTY", 20, "1m")`, "expected 5m"},
		{"fifteen-minute timeframe", `chart.ema("NIFTY", 20, "15m")`, "expected 5m"},
		{"daily timeframe", `chart.ema("NIFTY", 20, "1d")`, "expected 5m"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			thread := &starlark.Thread{Name: "chart-test"}
			thread.SetLocal("strategy_context", &strategyContext{
				ctx:       context.Background(),
				execution: &engine.Execution{Time: time.Date(2026, 1, 1, 9, 25, 0, 0, time.UTC)},
			})
			_, err := starlark.Eval(thread, "chart.star", tt.expression, scriptBuiltins())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v; want %q", err, tt.want)
			}
		})
	}
}

func TestChartEMARequiresSimulation(t *testing.T) {
	_, err := starlark.Eval(&starlark.Thread{Name: "chart-test"}, "chart.star", `chart.ema("NIFTY", 20, "5m")`, scriptBuiltins())
	if err == nil || !strings.Contains(err.Error(), "only available during on_candle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChartEMAUsesCallbackContext(t *testing.T) {
	f := newFixture(t)
	script, err := LoadSource("chart.star", setup+`
def on_candle(ctx):
    chart.ema("NIFTY", 20, "5m")
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = script.Run(context.Background(), engine.NewEngine(f.repo))
	if err == nil || !strings.Contains(err.Error(), "EMA not implemented") {
		t.Fatalf("unexpected error: %v", err)
	}
}
