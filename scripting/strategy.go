// Package scripting adapts Starlark strategy callbacks to the Go engine.
package scripting

import (
	"context"
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"optionsbacktest/engine"
	"optionsbacktest/models"
	"optionsbacktest/selector"
)

type Script struct {
	thread   *starlark.Thread
	onCandle starlark.Callable
	context  *strategyContext
}

type strategyContext struct {
	initializing bool
	configured   bool
	underlying   string
	start, end   time.Time
	execution    *engine.Execution
	state        *starlark.Dict
}

// Load compiles and evaluates the file once, then calls initialize once.
func Load(filename string) (*Script, error) {
	return load(filename, nil)
}

// LoadSource compiles and initializes a strategy directly from its stored source.
// The filename is a diagnostic label; no file is read or written.
func LoadSource(filename, source string) (*Script, error) {
	return load(filename, source)
}

func load(filename string, source any) (*Script, error) {
	thread := &starlark.Thread{Name: filename}
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, filename, source, starlark.StringDict{
		"offset_selector": starlark.NewBuiltin("offset_selector", offsetSelector),
		"id_selector":     starlark.NewBuiltin("id_selector", idSelector),
	})
	if err != nil {
		return nil, fmt.Errorf("load strategy: %w", err)
	}
	initialize, ok := globals["initialize"].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("strategy must define initialize(ctx)")
	}
	onCandle, ok := globals["on_candle"].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("strategy must define on_candle(ctx)")
	}
	globals.Freeze()
	c := &strategyContext{initializing: true, state: starlark.NewDict(0)}
	if _, err := starlark.Call(thread, initialize, starlark.Tuple{c}, nil); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	c.initializing = false
	if !c.configured {
		return nil, fmt.Errorf("initialize must configure the strategy")
	}
	return &Script{thread: thread, onCandle: onCandle, context: c}, nil
}

// Run passes script configuration and callbacks to the engine. Each engine
// expiry has independent state, orders, and positions; the script globals stay frozen.
func (s *Script) Run(ctx context.Context, e *engine.Engine) (*engine.BacktestResult, error) {
	defer func() { s.context.execution = nil }()
	return e.Backtest(ctx, s.context.underlying, s.context.start, s.context.end, func(execution *engine.Execution) error {
		if s.context.execution != execution {
			s.context.state = starlark.NewDict(0)
		}
		s.context.execution = execution
		_, err := starlark.Call(s.thread, s.onCandle, starlark.Tuple{s.context}, nil)
		if err != nil {
			return fmt.Errorf("on_candle: %w", err)
		}
		return nil
	})
}

func (*strategyContext) String() string       { return "strategy_context" }
func (*strategyContext) Type() string         { return "strategy_context" }
func (*strategyContext) Freeze()              {}
func (*strategyContext) Truth() starlark.Bool { return starlark.True }
func (*strategyContext) Hash() (uint32, error) {
	return 0, fmt.Errorf("strategy context is unhashable")
}
func (*strategyContext) AttrNames() []string {
	return []string{"configure", "place_order", "order", "state", "time", "dte"}
}

func (c *strategyContext) Attr(name string) (starlark.Value, error) {
	switch name {
	case "configure":
		return starlark.NewBuiltin(name, c.configure), nil
	case "place_order":
		return starlark.NewBuiltin(name, c.placeOrder), nil
	case "order":
		return starlark.NewBuiltin(name, c.order), nil
	case "state":
		return c.state, nil
	case "dte":
		if c.execution != nil {
			return starlark.MakeInt(c.execution.DTE), nil
		}
		return starlark.None, nil
	case "time":
		if c.execution != nil {
			return starlark.String(c.execution.Time.Format("15:04")), nil
		}
		return starlark.None, nil

	}
	return nil, nil
}

func (c *strategyContext) configure(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if !c.initializing || c.configured {
		return nil, fmt.Errorf("configure is allowed once during initialize")
	}
	var underlying, start, end string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "underlying", &underlying, "start_date", &start, "end_date", &end); err != nil {
		return nil, err
	}
	startDate, err := time.Parse(time.DateOnly, start)
	if err != nil {
		return nil, fmt.Errorf("start_date: %w", err)
	}
	endDate, err := time.Parse(time.DateOnly, end)
	if err != nil {
		return nil, fmt.Errorf("end_date: %w", err)
	}
	if underlying == "" || startDate.After(endDate) {
		return nil, fmt.Errorf("underlying must be nonempty and start_date must not follow end_date")
	}
	c.underlying, c.start, c.end, c.configured = underlying, startDate, endDate, true
	return starlark.None, nil
}

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
		return nil, fmt.Errorf("contract must be offset_selector(...) or id_selector(...)")
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
	return &selectorValue{selector: selector.NewOffsetSelector(models.OptionType(optionType), offset), name: b.Name()}, nil
}

func idSelector(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id int64
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "id", &id); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, fmt.Errorf("contract ID must be a positive int64")
	}
	return &selectorValue{selector: selector.NewIDSelector(id), name: b.Name()}, nil
}
