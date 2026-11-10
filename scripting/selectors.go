package scripting

import (
	"fmt"

	"go.starlark.net/starlark"

	"optionsbacktest/models"
	"optionsbacktest/selector"
)

// selectorValue exposes a Go selector as an immutable Starlark value.
// Construction does not access market data; the engine calls Select later.
type selectorValue struct {
	selector selector.Selector
	name     string
}

func (s *selectorValue) String() string {
	return s.name + "(...)"
}
func (s *selectorValue) Type() string       { return s.name }
func (*selectorValue) Freeze()              {}
func (*selectorValue) Truth() starlark.Bool { return starlark.True }
func (*selectorValue) Hash() (uint32, error) {
	return 0, fmt.Errorf("contract selectors are unhashable")
}

func offsetSelector(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var optionType string
	var offset int
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "option_type", &optionType, "offset?", &offset); err != nil {
		return nil, err
	}
	if optionType != string(models.Call) && optionType != string(models.Put) {
		return nil, fmt.Errorf("invalid option_type %q: expected CALL or PUT", optionType)
	}
	return wrapSelector(b.Name(), selector.NewOffsetSelector(models.OptionType(optionType), offset)), nil
}

func idSelector(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id int64
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "id", &id); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, fmt.Errorf("contract ID must be a positive int64")
	}
	return wrapSelector(b.Name(), selector.NewIDSelector(id)), nil
}

func deltaSelector(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var optionType string
	var delta float64
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "option_type", &optionType, "delta", &delta); err != nil {
		return nil, err
	}
	if optionType != string(models.Call) && optionType != string(models.Put) {
		return nil, fmt.Errorf("invalid option_type %q: expected CALL or PUT", optionType)
	}
	return wrapSelector(b.Name(), selector.NewDeltaSelector(models.OptionType(optionType), delta)), nil
}

func selectorBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"offset_selector": starlark.NewBuiltin("offset_selector", offsetSelector),
		"id_selector":     starlark.NewBuiltin("id_selector", idSelector),
		"delta_selector":  starlark.NewBuiltin("delta_selector", deltaSelector),
	}
}

func wrapSelector(name string, s selector.Selector) starlark.Value {
	return &selectorValue{selector: s, name: name}
}
