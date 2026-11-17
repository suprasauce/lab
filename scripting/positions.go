package scripting

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"optionsbacktest/models"
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
		var exitPrice, currentPrice starlark.Value = starlark.None, starlark.None
		if position.Status == models.TradeClosed {
			exitPrice = starlark.Float(float64(position.ExitPrice) / 100)
		} else if position.Candle != nil {
			currentPrice = starlark.Float(float64(position.Candle.Close) / 100)
		}
		value := starlarkstruct.FromStringDict(starlark.String("position"), starlark.StringDict{
			"status":      starlark.String(position.Status),
			"quantity":    starlark.MakeInt64(position.Quantity),
			"entry_price": starlark.Float(float64(position.EntryPrice) / 100),
			"exit_price":  exitPrice,
			"curr_price":  currentPrice,
		})
		value.Freeze()
		values = append(values, value)
	}
	return starlark.NewList(values), nil
}
