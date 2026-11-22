package scripting

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"optionsbacktest/models"
)

func ema(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var symbol, timeframe string
	var period int
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "tradingsymbol", &symbol, "period", &period, "timeframe", &timeframe); err != nil {
		return nil, err
	}
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("tradingsymbol must be nonempty")
	}
	if period <= 0 {
		return nil, fmt.Errorf("period must be positive")
	}
	if models.Timeframe(timeframe) != models.Timeframe5m {
		return nil, fmt.Errorf("unsupported timeframe %q: expected 5m", timeframe)
	}
	c, ok := thread.Local("strategy_context").(*strategyContext)
	if !ok || c.initializing || c.execution == nil {
		return nil, fmt.Errorf("chart.ema is only available during on_candle")
	}
	value, err := c.execution.ChartService.EMA(c.ctx, symbol, period, models.Timeframe(timeframe), c.execution.Time)
	if err != nil {
		return nil, err
	}
	return starlark.Float(value), nil
}
