package scripting

import (
	"testing"

	"go.starlark.net/starlark"
	"optionsbacktest/engine"
	"optionsbacktest/models"
)

func positionContext() *strategyContext {
	return &strategyContext{execution: &engine.Execution{Positions: []*engine.Position{
		{TradingSymbol: "CE", Trade: models.Trade{Status: models.TradeClosed, Quantity: -75, EntryPrice: 15050, ExitPrice: 11025}, Candle: &models.Candle{Close: 99999}},
		{TradingSymbol: "PE", Trade: models.Trade{Status: models.TradeOpen, Quantity: -75}},
		{TradingSymbol: "CE", Trade: models.Trade{Status: models.TradeOpen, Quantity: 75, EntryPrice: 12025}, Candle: &models.Candle{Close: 13050}},
	}}}
}

func TestGetPositions(t *testing.T) {
	c := positionContext()
	_, err := starlark.ExecFile(&starlark.Thread{}, "positions.star", `
def check(ctx):
    positions = ctx.get_positions(tradingsymbol="CE")
    if len(positions) != 2:
        fail("incorrect symbol filter")
    closed, opened = positions
    missing = ctx.get_positions("PE")[0]
    if closed.status != "CLOSED" or closed.quantity != -75 or closed.entry_price != 150.50 or closed.exit_price != 110.25 or closed.curr_price != None:
        fail("incorrect closed position")
    if opened.status != "OPEN" or opened.quantity != 75 or opened.entry_price != 120.25 or opened.exit_price != None or opened.curr_price != 130.50:
        fail("incorrect open position")
    if missing.quantity != -75 or missing.curr_price != None or missing.exit_price != None:
        fail("incorrect missing candle handling")
    if ctx.get_positions("unknown") != []:
        fail("expected empty list")
    positions.clear()
    if len(ctx.get_positions("CE")) != 2:
        fail("list mutation modified execution")
check(ctx)
`, starlark.StringDict{"ctx": c})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetPositionsErrors(t *testing.T) {
	for _, expression := range []string{
		`ctx.get_positions()`,
		`ctx.get_positions(123)`,
		`ctx.get_positions("CE")[0].quantity = 10`,
	} {
		_, err := starlark.ExecFile(&starlark.Thread{}, "positions.star", expression, starlark.StringDict{"ctx": positionContext()})
		if err == nil {
			t.Fatalf("expected error for %s", expression)
		}
	}
	for _, c := range []*strategyContext{{}, {initializing: true, execution: &engine.Execution{}}} {
		_, err := starlark.Eval(&starlark.Thread{}, "positions.star", `ctx.get_positions("CE")`, starlark.StringDict{"ctx": c})
		if err == nil {
			t.Fatal("positions exposed outside on_candle")
		}
	}
}
