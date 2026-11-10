package scripting

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func (c *strategyContext) placeOrder(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("%s is only available during on_candle", b.Name())
	}
	var contract starlark.Value
	var side string
	var quantity int64
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "contract", &contract, "side", &side, "quantity", &quantity); err != nil {
		return nil, err
	}
	selection, ok := contract.(*selectorValue)
	if !ok {
		return nil, fmt.Errorf("contract must be a selector, got %s", contract.Type())
	}
	id, err := c.execution.PlaceOrder(selection.selector, side, quantity)
	if err != nil {
		return nil, err
	}
	return starlark.String(id), nil
}

func (c *strategyContext) order(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("order is only available during on_candle")
	}
	var id string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "order_id", &id); err != nil {
		return nil, err
	}
	order, err := c.execution.Order(id)
	if err != nil {
		return nil, err
	}
	var contractID starlark.Value = starlark.None
	if order.Contract != nil {
		contractID = starlark.MakeInt64(order.Contract.ID)
	}
	result := starlarkstruct.FromStringDict(starlark.String("order"), starlark.StringDict{
		"id": starlark.String(order.ID), "status": starlark.String(order.Status), "contract_id": contractID,
	})
	result.Freeze()
	return result, nil
}
