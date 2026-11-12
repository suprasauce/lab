// Package scripting adapts Starlark strategy callbacks to the Go engine.
package scripting

import (
	"context"
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"optionsbacktest/engine"
)

type Script struct {
	thread     *starlark.Thread
	initialize starlark.Callable
	onCandle   starlark.Callable
	context    *strategyContext
}

// Load compiles and evaluates the file once.
func Load(filename string) (*Script, error) {
	return load(filename, nil)
}

// LoadSource compiles a strategy directly from its stored source.
// The filename is a diagnostic label; no file is read or written.
func LoadSource(filename, source string) (*Script, error) {
	return load(filename, source)
}

func load(filename string, source any) (*Script, error) {
	thread := &starlark.Thread{Name: filename}
	globals, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, filename, source, selectorBuiltins())
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
	return &Script{thread: thread, initialize: initialize, onCandle: onCandle}, nil
}

// Run calls initialize once; the script synchronously chooses each expiry run.
func (s *Script) Run(ctx context.Context, e *engine.Engine) (*engine.BacktestResult, error) {
	result := &engine.BacktestResult{Executions: make([][]*engine.Position, 0)}
	c := &strategyContext{initializing: true, state: starlark.NewDict(0)}
	s.context = c
	callbackThread := &starlark.Thread{Name: s.thread.Name + ":on_candle"}
	c.getExpiries = func(start, end time.Time) ([]time.Time, error) {
		return e.GetExpiries(ctx, c.underlying, start, end)
	}
	c.simulateExpiry = func(expiry time.Time) error {
		c.state = starlark.NewDict(0)
		c.initializing = false
		defer func() { c.initializing = true; c.execution = nil }()
		cycle, err := e.Simulate(ctx, c.underlying, expiry, func(execution *engine.Execution) error {
			c.execution = execution
			_, err := starlark.Call(callbackThread, s.onCandle, starlark.Tuple{c}, nil)
			if err != nil {
				return fmt.Errorf("on_candle: %w", err)
			}
			return nil
		})
		if cycle != nil {
			result.Executions = append(result.Executions, cycle.Executions...)
		}
		return err
	}
	defer func() { c.initializing = false; c.execution = nil }()
	if _, err := starlark.Call(s.thread, s.initialize, starlark.Tuple{c}, nil); err != nil {
		return result, fmt.Errorf("initialize: %w", err)
	}
	if !c.configured {
		return result, fmt.Errorf("initialize must configure the strategy")
	}
	result.Underlying = c.underlying
	return result, nil
}
