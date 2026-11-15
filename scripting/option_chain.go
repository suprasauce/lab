package scripting

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"optionsbacktest/optionchain"
)

// filterValue is an immutable selection request; construction performs no queries.
type filterValue struct {
	optionchain.OptionFilter
}

func (f *filterValue) String() string {
	return fmt.Sprintf("filter(type=%q, value=%g)", f.OptionFilter.Type, f.Value)
}
func (*filterValue) Type() string          { return "filter" }
func (*filterValue) Freeze()               {}
func (*filterValue) Truth() starlark.Bool  { return starlark.True }
func (*filterValue) Hash() (uint32, error) { return 0, fmt.Errorf("filters are unhashable") }

func newFilter(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var filter optionchain.OptionFilter
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "type", &filter.Type, "value", &value); err != nil {
		return nil, err
	}
	var ok bool
	filter.Value, ok = starlark.AsFloat(value)
	if !ok {
		return nil, fmt.Errorf("filter value must be a number, got %s", value.Type())
	}
	switch filter.Type {
	case "delta", "offset", "price":
	default:
		return nil, fmt.Errorf("invalid filter type %q: expected delta, offset, or price", filter.Type)
	}
	return &filterValue{OptionFilter: filter}, nil
}

func (c *strategyContext) optionChain(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var underlying, expiry, optionType string
	var filter starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"underlying", &underlying, "expiry", &expiry, "option_type", &optionType, "filter", &filter); err != nil {
		return nil, err
	}
	if underlying == "" {
		return nil, fmt.Errorf("underlying must be nonempty")
	}
	expiryDate, err := time.Parse(time.DateOnly, expiry)
	if err != nil {
		return nil, fmt.Errorf("expiry: %w", err)
	}
	if optionType != "CE" && optionType != "PE" {
		return nil, fmt.Errorf("invalid option_type %q: expected CE or PE", optionType)
	}
	selection, ok := filter.(*filterValue)
	if !ok {
		return nil, fmt.Errorf("filter must be constructed with filter(type=..., value=...)")
	}
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("option_chain is only available during on_candle")
	}
	service := optionchain.NewOptionChainService(c.repo)
	symbol, err := service.GetSymbol(c.ctx, underlying, expiryDate, optionType,
		selection.OptionFilter, c.execution.Time)
	if err != nil {
		return nil, err
	}
	if symbol == "" {
		return starlark.None, nil
	}
	return starlark.String(symbol), nil
}
