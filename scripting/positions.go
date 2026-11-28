package scripting

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func (c *strategyContext) getPositions(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("get_positions is only available during on_candle")
	}
	var tradingSymbol string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "tradingsymbol", &tradingSymbol); err != nil {
		return nil, err
	}
	positions := c.execution.GetPositions(tradingSymbol)
	values := make([]starlark.Value, 0, len(positions))
	for _, position := range positions {
		var candle starlark.Value = starlark.None
		status := "CLOSED"
		if position.Quantity != 0 {
			status = "OPEN"
			if position.Candle != nil {
				candle = candleValue(position.Candle, c.execution.Time.Location())
			}
		}
		value := starlarkstruct.FromStringDict(starlark.String("position"), starlark.StringDict{
			"status":        starlark.String(status),
			"simulation_id": starlark.String(position.SimulationID),
			"realized_pnl":  starlark.Float(float64(position.RealizedPnL) / 100),
			"quantity":      starlark.MakeInt64(position.Quantity),
			"average_price": starlark.Float(float64(position.AveragePrice) / 100),
			"candle":        candle,
		})
		value.Freeze()
		values = append(values, value)
	}
	return starlark.NewList(values), nil
}

func (c *strategyContext) getLotSize(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("get_lot_size is only available during on_candle")
	}
	var symbol string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "tradingsymbol", &symbol); err != nil {
		return nil, err
	}
	value, err := c.execution.GetLotSize(symbol)
	if err != nil {
		return nil, err
	}
	return starlark.MakeInt64(value), nil
}
