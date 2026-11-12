package scripting

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"

	"optionsbacktest/engine"
)

type strategyContext struct {
	initializing   bool
	configured     bool
	underlying     string
	getExpiries    func(time.Time, time.Time) ([]time.Time, error)
	simulateExpiry func(time.Time) error
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
	return []string{"configure", "get_expiries", "simulate", "place_order", "order", "state", "time", "dte"}
}

func (c *strategyContext) Attr(name string) (starlark.Value, error) {
	switch name {
	case "get_expiries":
		return starlark.NewBuiltin(name, c.expiries), nil
	case "simulate":
		return starlark.NewBuiltin(name, c.simulate), nil
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
	var underlying string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "underlying", &underlying); err != nil {
		return nil, err
	}
	if underlying == "" {
		return nil, fmt.Errorf("underlying must be nonempty")
	}
	c.underlying, c.configured = underlying, true
	return starlark.None, nil
}

func (c *strategyContext) expiries(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if !c.initializing || !c.configured {
		return nil, fmt.Errorf("get_expiries requires configure during initialize")
	}
	var start, end string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "start", &start, "end", &end); err != nil {
		return nil, err
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
	dates, err := c.getExpiries(startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("get expiries: %w", err)
	}
	values := make([]starlark.Value, len(dates))
	for i, date := range dates {
		values[i] = starlark.String(date.Format(time.DateOnly))
	}
	return starlark.NewList(values), nil
}

func (c *strategyContext) simulate(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if !c.initializing || !c.configured {
		return nil, fmt.Errorf("simulate requires configure during initialize")
	}
	var expiry string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "expiry", &expiry); err != nil {
		return nil, err
	}
	date, err := time.Parse(time.DateOnly, expiry)
	if err != nil {
		return nil, fmt.Errorf("expiry: %w", err)
	}
	if err := c.simulateExpiry(date); err != nil {
		return nil, err
	}
	return starlark.None, nil
}
