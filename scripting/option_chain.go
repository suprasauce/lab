package scripting

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
)

// filterValue is an immutable selection request; construction performs no queries.
type filterValue struct {
	kind  string
	value starlark.Value
}

func (f *filterValue) String() string {
	return fmt.Sprintf("filter(type=%q, value=%s)", f.kind, f.value)
}
func (*filterValue) Type() string          { return "filter" }
func (*filterValue) Freeze()               {}
func (*filterValue) Truth() starlark.Bool  { return starlark.True }
func (*filterValue) Hash() (uint32, error) { return 0, fmt.Errorf("filters are unhashable") }

func newFilter(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var kind string
	var value starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "type", &kind, "value", &value); err != nil {
		return nil, err
	}
	switch kind {
	case "delta", "strike", "price":
	default:
		return nil, fmt.Errorf("invalid filter type %q: expected delta, strike, or price", kind)
	}
	switch value.(type) {
	case starlark.Int, starlark.Float:
	default:
		return nil, fmt.Errorf("filter value must be a number, got %s", value.Type())
	}
	return &filterValue{kind: kind, value: value}, nil
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
	if _, err := time.Parse(time.DateOnly, expiry); err != nil {
		return nil, fmt.Errorf("expiry: %w", err)
	}
	if optionType != "CE" && optionType != "PE" {
		return nil, fmt.Errorf("invalid option_type %q: expected CE or PE", optionType)
	}
	if _, ok := filter.(*filterValue); !ok {
		return nil, fmt.Errorf("filter must be constructed with filter(type=..., value=...)")
	}
	// TODO: Select a contract and return its trading symbol, or None if unavailable.
	return nil, fmt.Errorf("option_chain is not implemented")
}
