package scripting

import (
	"context"
	"strings"
	"testing"

	"optionsbacktest/engine"
)

func TestCurrentCandleAndImmediatePriceExecution(t *testing.T) {
	f := newFixture(t)
	script, err := LoadSource("immediate.star", `
def initialize(ctx):
    ctx.configure()
    ctx.simulate(start_date="2026-08-15", end_date="2026-08-15")

def on_candle(ctx):
    if ctx.time != "09:19":
        return
    candle = ctx.get_candle("NIFTY")
    if candle.timestamp != "2026-08-15T09:19:00+05:30":
        fail("expected current-minute candle")
    if candle.open != 22530 or candle.high != 22531 or candle.low != 22529 or candle.close != 22530.5:
        fail("incorrect full OHLC")
    entry = ctx.place_trade(tradingsymbol="NIFTY", quantity=2, price=candle.open)
    if entry != "1/1" or ctx.get_positions("NIFTY")[0].quantity != 2:
        fail("entry not immediately visible")
    if ctx.get_positions("NIFTY")[0].candle.timestamp != candle.timestamp:
        fail("position did not receive current candle")
    ctx.place_trade(tradingsymbol="NIFTY", quantity=-2, price=candle.close)
    position = ctx.get_positions("NIFTY")[0]
    if position.quantity != 0 or position.realized_pnl != 1:
        fail("exit accounting not immediately visible")
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
	if err != nil {
		t.Fatal(err)
	}
	p := result.Executions[0].Positions[0]
	if len(p.Trades) != 2 || p.Trades[0].Timestamp != p.Trades[1].Timestamp || p.RealizedPnL != 100 {
		t.Fatal("incorrect immediate trade ledger")
	}
}

func TestOrderPriceValidation(t *testing.T) {
	for _, call := range []string{
		`ctx.place_trade(tradingsymbol="NIFTY", quantity=1)`,
		`ctx.place_trade(tradingsymbol="NIFTY", quantity=1, price="100")`,
		`ctx.place_trade(tradingsymbol="NIFTY", quantity=1, price=-1)`,
		`ctx.place_trade(tradingsymbol="NIFTY", quantity=1, price=1e100)`,
	} {
		t.Run(call, func(t *testing.T) {
			f := newFixture(t)
			script, err := LoadSource("invalid.star", setup+"\ndef on_candle(ctx):\n    "+call+"\n")
			if err != nil {
				t.Fatal(err)
			}
			_, err = script.Run(context.Background(), engine.NewEngine(f.repo))
			if err == nil || !strings.Contains(err.Error(), "price") {
				t.Fatalf("invalid price accepted: %v", err)
			}
		})
	}
}
