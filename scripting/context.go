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
	ctx           context.Context
	repo          *repository.Repository
	initializing  bool
	configured    bool
	getExpiries   func(string, time.Time, time.Time) ([]time.Time, error)
	simulateRange func(time.Time, time.Time) error
	execution     *engine.Execution
	state         *starlark.Dict
}

func (*strategyContext) String() string       { return "strategy_context" }
func (*strategyContext) Type() string         { return "strategy_context" }
func (*strategyContext) Freeze()              {}
func (*strategyContext) Truth() starlark.Bool { return starlark.True }
func (*strategyContext) Hash() (uint32, error) {
	return 0, fmt.Errorf("strategy context is unhashable")
}
func (*strategyContext) AttrNames() []string {
	return []string{"configure", "get_candle", "get_lot_size", "option_chain", "get_expiries", "get_expiry", "simulate", "place_trade", "get_positions", "state", "clear_state", "time", "date"}
}

func (c *strategyContext) Attr(name string) (starlark.Value, error) {
	switch name {
	case "clear_state":
		return starlark.NewBuiltin(name, c.clearState), nil
	case "option_chain":
		return starlark.NewBuiltin(name, c.optionChain), nil
	case "get_expiries":
		return starlark.NewBuiltin(name, c.expiries), nil
	case "get_expiry":
		return starlark.NewBuiltin(name, c.getExpiry), nil
	case "simulate":
		return starlark.NewBuiltin(name, c.simulate), nil
	case "configure":
		return starlark.NewBuiltin(name, c.configure), nil
	case "get_candle":
		return starlark.NewBuiltin(name, c.getCandle), nil
	case "get_lot_size":
		return starlark.NewBuiltin(name, c.getLotSize), nil
	case "place_trade":
		return starlark.NewBuiltin(name, c.placeTrade), nil
	case "get_positions":
		return starlark.NewBuiltin(name, c.getPositions), nil
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
	var start, end string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "start_date", &start, "end_date", &end); err != nil {
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
	if startDate.After(endDate) {
		return nil, fmt.Errorf("start_date must not be after end_date")
	}
	if err := c.simulateRange(startDate, endDate); err != nil {
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
