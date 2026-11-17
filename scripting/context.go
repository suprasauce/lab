package scripting

import (
	"context"
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"optionsbacktest/engine"
	"optionsbacktest/repository"
)

type strategyContext struct {
	ctx            context.Context
	repo           *repository.Repository
	initializing   bool
	configured     bool
	getExpiries    func(string, time.Time, time.Time) ([]time.Time, error)
	simulateExpiry func(string, time.Time) error
	execution      *engine.Execution
	state          *starlark.Dict
}

func (*strategyContext) String() string       { return "strategy_context" }
func (*strategyContext) Type() string         { return "strategy_context" }
func (*strategyContext) Freeze()              {}
func (*strategyContext) Truth() starlark.Bool { return starlark.True }
func (*strategyContext) Hash() (uint32, error) {
	return 0, fmt.Errorf("strategy context is unhashable")
}
func (*strategyContext) AttrNames() []string {
	return []string{"configure", "option_chain", "get_expiries", "simulate", "place_order", "order", "get_positions", "state", "clear_state", "time", "date"}
}

func (c *strategyContext) Attr(name string) (starlark.Value, error) {
	switch name {
	case "clear_state":
		return starlark.NewBuiltin(name, c.clearState), nil
	case "option_chain":
		return starlark.NewBuiltin(name, c.optionChain), nil
	case "get_expiries":
		return starlark.NewBuiltin(name, c.expiries), nil
	case "simulate":
		return starlark.NewBuiltin(name, c.simulate), nil
	case "configure":
		return starlark.NewBuiltin(name, c.configure), nil
	case "place_order":
		return starlark.NewBuiltin(name, c.placeOrder), nil
	case "get_positions":
		return starlark.NewBuiltin(name, c.getPositions), nil
	case "order":
		return starlark.NewBuiltin(name, c.order), nil
	case "state":
		return c.state, nil
	case "date":
		if c.execution != nil {
			return starlark.String(c.execution.Time.Format(time.DateOnly)), nil
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
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	c.configured = true
	return starlark.None, nil
}

func (c *strategyContext) expiries(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if !c.initializing || !c.configured {
		return nil, fmt.Errorf("get_expiries requires configure during initialize")
	}
	var underlying, start, end string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "underlying", &underlying, "start", &start, "end", &end); err != nil {
		return nil, err
	}
	if underlying == "" {
		return nil, fmt.Errorf("underlying must be nonempty")
	}
	startDate, err := time.Parse(time.DateOnly, start)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	endDate, err := time.Parse(time.DateOnly, end)
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	if startDate.After(endDate) {
		return nil, fmt.Errorf("start_date must not be after end date")
	}
	dates, err := c.getExpiries(underlying, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("get expiries: %w", err)
	}
	values := make([]starlark.Value, len(dates))
	for i, date := range dates {
		values[i] = &expiryValue{date: date, underlying: underlying}
	}
	return starlark.NewList(values), nil
}

func (c *strategyContext) simulate(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if !c.initializing || !c.configured {
		return nil, fmt.Errorf("simulate requires configure during initialize")
	}
	var expiry starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "expiry", &expiry); err != nil {
		return nil, err
	}
	value, ok := expiry.(*expiryValue)
	if !ok {
		return nil, fmt.Errorf("expiry must be an expiry object returned by get_expiries")
	}
	if err := c.simulateExpiry(value.underlying, value.date); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (c *strategyContext) clearState(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	if err := c.state.Clear(); err != nil {
		return nil, fmt.Errorf("clear state: %w", err)
	}
	return starlark.None, nil
}
