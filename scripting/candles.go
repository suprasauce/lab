package scripting

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"optionsbacktest/models"
)

func (c *strategyContext) getCandle(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("get_candle is only available during on_candle")
	}
	var symbol string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "tradingsymbol", &symbol); err != nil {
		return nil, err
	}
	candle, err := c.execution.GetCandle(symbol)
	if err != nil {
		return nil, err
	}
	return candleValue(candle, c.execution.Time.Location()), nil
}

func candleValue(candle *models.Candle, location *time.Location) starlark.Value {
	if candle == nil {
		return starlark.None
	}
	value := starlarkstruct.FromStringDict(starlark.String("candle"), starlark.StringDict{
		"timestamp": starlark.String(candle.Timestamp.In(location).Format(time.RFC3339)),
		"open":      starlark.Float(float64(candle.Open) / 100),
		"high":      starlark.Float(float64(candle.High) / 100),
		"low":       starlark.Float(float64(candle.Low) / 100),
		"close":     starlark.Float(float64(candle.Close) / 100),
	})
	value.Freeze()
	return value
}
