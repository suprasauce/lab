package scripting

import (
	"testing"

	"go.starlark.net/starlark"
	"optionsbacktest/engine"
	"optionsbacktest/models"
)

func positionContext() *strategyContext {
	return &strategyContext{execution: &engine.Execution{Positions: []*models.Position{
		{TradingSymbol: "CLOSED", SimulationID: "1", Quantity: 0, AveragePrice: 15050, RealizedPnL: 10000, Candle: &models.Candle{Close: 99999}},
		{TradingSymbol: "PE", Quantity: -75},
		{TradingSymbol: "CE", SimulationID: "1", Quantity: 75, AveragePrice: 12025, Candle: &models.Candle{Close: 13050}},
	}}}
}

func TestGetPositions(t *testing.T) {
	c := positionContext()
	_, err := starlark.ExecFile(&starlark.Thread{}, "positions.star", `
def check(ctx):
    positions = ctx.get_positions(tradingsymbol="CE")
    if len(positions) != 1:
        fail("incorrect symbol filter")
    closed = ctx.get_positions("CLOSED")[0]
    opened = positions[0]
    missing = ctx.get_positions("PE")[0]
    if closed.simulation_id != "1" or closed.realized_pnl != 100 or closed.status != "CLOSED" or closed.quantity != 0 or closed.average_price != 150.50 or closed.curr_price != None:
        fail("incorrect closed position")
    if opened.status != "OPEN" or opened.quantity != 75 or opened.average_price != 120.25 or opened.curr_price != 130.50:
        fail("incorrect open position")
    if missing.quantity != -75 or missing.curr_price != None:
        fail("incorrect missing candle handling")
    if ctx.get_positions("unknown") != []:
        fail("expected empty list")
    positions.clear()
    if len(ctx.get_positions("CE")) != 1:
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
		`ctx.get_positions("CE")[0].unrealized_pnl`,
		`ctx.get_positions("CE")[0].entry_price`,
		`ctx.get_positions("CE")[0].exit_price`,
		`ctx.get_positions("CE")[0].quantity = 10`,
		`ctx.get_positions("CE")[0].entry_price`,
		`ctx.get_positions("CE")[0].exit_price`,
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
