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
	legs         []models.OptionLeg
	execution    *engine.Execution
}

// Load compiles and evaluates the file once, then calls initialize once.
func Load(filename string) (*Script, error) {
	return load(filename, nil)
}

func load(filename string, source any) (*Script, error) {
	thread := &starlark.Thread{Name: filename}
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, filename, source, nil)
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
	c := &strategyContext{initializing: true}
	if _, err := starlark.Call(thread, initialize, starlark.Tuple{c}, nil); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	c.initializing = false
	if !c.configured || len(c.legs) == 0 {
		return nil, fmt.Errorf("initialize must configure the strategy and register at least one leg")
	}
	return &Script{thread: thread, onCandle: onCandle, context: c}, nil
}

// Run passes script configuration and callbacks to the engine. Each engine
// expiry has independent leg executions; the script globals stay frozen.
func (s *Script) Run(ctx context.Context, e *engine.Engine) ([][]models.LegExecution, error) {
	defer func() { s.context.execution = nil }()
	return e.Backtest(ctx, s.context.underlying, s.context.start, s.context.end, s.context.legs, func(execution *engine.Execution) error {
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
	return []string{"configure", "register_leg", "sell_call", "close", "leg", "dte", "underlying_candle", "legs"}
}

func (c *strategyContext) Attr(name string) (starlark.Value, error) {
	switch name {
	case "configure":
		return starlark.NewBuiltin(name, c.configure), nil
	case "register_leg":
		return starlark.NewBuiltin(name, c.registerLeg), nil
	case "sell_call", "close", "leg":
		return starlark.NewBuiltin(name, c.operation), nil
	case "dte":
		if c.execution != nil {
			return starlark.MakeInt(c.execution.DTE), nil
		}
		return starlark.None, nil
	case "underlying_candle":
		if c.execution != nil {
			return candleValue(&c.execution.UnderlyingCandle), nil
		}
		return starlark.None, nil
	case "legs":
		dict := starlark.NewDict(len(c.legs))
		if c.execution != nil {
			for _, leg := range c.execution.Legs {
				if err := dict.SetKey(starlark.String(leg.Leg.ID), legValue(leg)); err != nil {
					return nil, err
				}
			}
		}
		dict.Freeze()
		return dict, nil
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

func (c *strategyContext) registerLeg(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if !c.initializing {
		return nil, fmt.Errorf("register_leg is only allowed during initialize")
	}
	var name string
	var quantity int64
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "quantity", &quantity); err != nil {
		return nil, err
	}
	if name == "" || quantity <= 0 {
		return nil, fmt.Errorf("leg name must be nonempty and quantity must be positive")
	}
	for _, leg := range c.legs {
		if leg.ID == name {
			return nil, fmt.Errorf("leg %q already registered", name)
		}
	}
	c.legs = append(c.legs, models.OptionLeg{ID: name, Quantity: quantity})
	return starlark.None, nil
}

func (c *strategyContext) operation(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("%s is only available during on_candle", b.Name())
	}
	var name string
	var offset int
	var err error
	if b.Name() == "sell_call" {
		err = starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "offset?", &offset)
	} else {
		err = starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name)
	}
	if err != nil {
		return nil, err
	}
	switch b.Name() {
	case "sell_call":
		err = c.execution.SellCall(name, offset)
	case "close":
		err = c.execution.Close(name)
	case "leg":
		leg, lookupErr := c.execution.Leg(name)
		if lookupErr != nil {
			return nil, lookupErr
		}
		return legValue(leg), nil
	}
	if err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func candleValue(c *models.Candle) starlark.Value {
	if c == nil {
		return starlark.None
	}
	return starlarkstruct.FromStringDict(starlark.String("candle"), starlark.StringDict{
		"open": starlark.MakeInt64(int64(c.Open)), "high": starlark.MakeInt64(int64(c.High)),
		"low": starlark.MakeInt64(int64(c.Low)), "close": starlark.MakeInt64(int64(c.Close)),
		"start_time": starlark.String(c.Timestamp.Format(time.RFC3339)),
	})
}

func legValue(leg *engine.Leg) starlark.Value {
	var contract, entryPrice starlark.Value = starlark.None, starlark.None
	if leg.Contract != nil {
		contract = starlarkstruct.FromStringDict(starlark.String("option"), starlark.StringDict{
			"id":           starlark.MakeInt64(leg.Contract.ID),
			"strike_price": starlark.MakeInt64(int64(leg.Contract.StrikePrice)),
			"expiry":       starlark.String(leg.Contract.Expiry.Format(time.DateOnly)),
			"option_type":  starlark.String(leg.Contract.OptionType),
		})
	}
	if len(leg.Trades) != 0 {
		entryPrice = starlark.MakeInt64(int64(leg.Trades[len(leg.Trades)-1].EntryPrice))
	}
	return starlarkstruct.FromStringDict(starlark.String("leg"), starlark.StringDict{
		"status": starlark.String(leg.Status), "selected_contract": contract,
		"entry_price": entryPrice, "quantity": starlark.MakeInt64(leg.Leg.Quantity),
		"option_candle": candleValue(leg.Candle),
	})
}
