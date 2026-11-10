// Package scripting adapts Starlark strategy callbacks to the Go engine.
package scripting

import (
	"context"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"optionsbacktest/engine"
)

type Script struct {
	thread   *starlark.Thread
	onCandle starlark.Callable
	context  *strategyContext
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
