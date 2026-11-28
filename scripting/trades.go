package scripting

import (
	"fmt"
	"math"
	"optionsbacktest/models"

	"go.starlark.net/starlark"
)

func (c *strategyContext) placeTrade(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("%s is only available during on_candle", b.Name())
	}
	var tradingSymbol string
	var quantity int64
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "tradingsymbol", &tradingSymbol, "quantity", &quantity, "price", &value); err != nil {
		return nil, err
	}
	price, ok := starlark.AsFloat(value)
	if !ok || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 || price*100 >= math.Exp2(63) {
		return nil, fmt.Errorf("price must be a finite nonnegative number within the supported range")
	}
	id, err := c.execution.PlaceTrade(tradingSymbol, quantity, models.Price(math.Round(price*100)))
	if err != nil {
		return nil, err
	}
	return starlark.String(id), nil
}
