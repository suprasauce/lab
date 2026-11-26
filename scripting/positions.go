package scripting

import (
	"fmt"
	"time"

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
				bar := position.Candle
				candle = starlarkstruct.FromStringDict(starlark.String("candle"), starlark.StringDict{
					"timestamp": starlark.String(bar.Timestamp.In(c.execution.Time.Location()).Format(time.RFC3339)),
					"open":      starlark.Float(float64(bar.Open) / 100),
					"high":      starlark.Float(float64(bar.High) / 100),
					"low":       starlark.Float(float64(bar.Low) / 100),
					"close":     starlark.Float(float64(bar.Close) / 100),
				})
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
