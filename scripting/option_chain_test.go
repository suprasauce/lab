package scripting

import (
	"context"
	"strings"
	"testing"
	"time"

	"optionsbacktest/engine"

	"go.starlark.net/starlark"
)

func TestFilterConstructor(t *testing.T) {
	for _, expression := range []string{
		`filter(type="delta", value=0.2)`,
		`filter(type="offset", value=2)`,
		`filter(type="price", value=150.50)`,
	} {
		value, err := starlark.Eval(&starlark.Thread{Name: "test"}, "test.star", expression, scriptBuiltins())
		if err != nil {
			t.Fatal(err)
		}
		filter, ok := value.(*filterValue)
		if !ok {
			t.Fatalf("unexpected value: %v", value)
		}
		filter.Freeze()
		if filter.OptionFilter.Type == "" || filter.Value == 0 {
			t.Fatal("filter configuration was lost")
		}
	}
}

func TestOptionChainArguments(t *testing.T) {
	for _, tt := range []struct{ name, expression, want string }{
		{"CE TODO", `ctx.option_chain(underlying="NIFTY", expiry="2026-01-27", option_type="CE", filter=filter(type="delta", value=0.2))`, "delta selection is not implemented"},
		{"price TODO", `ctx.option_chain(underlying="NIFTY", expiry="2026-01-27", option_type="CE", filter=filter(type="price", value=150))`, "price selection is not implemented"},
		{"missing underlying", `ctx.option_chain(expiry="2026-01-27", option_type="CE", filter=filter(type="delta", value=0.2))`, "underlying"},
		{"missing expiry", `ctx.option_chain(underlying="NIFTY", option_type="CE", filter=filter(type="delta", value=0.2))`, "expiry"},
		{"missing option type", `ctx.option_chain(underlying="NIFTY", expiry="2026-01-27", filter=filter(type="delta", value=0.2))`, "option_type"},
		{"missing filter", `ctx.option_chain(underlying="NIFTY", expiry="2026-01-27", option_type="CE")`, "filter"},
		{"empty underlying", `ctx.option_chain(underlying="", expiry="2026-01-27", option_type="CE", filter=filter(type="delta", value=0.2))`, "nonempty"},
		{"invalid expiry", `ctx.option_chain(underlying="NIFTY", expiry="bad", option_type="CE", filter=filter(type="delta", value=0.2))`, "expiry:"},
		{"invalid option type", `ctx.option_chain(underlying="NIFTY", expiry="2026-01-27", option_type="CALL", filter=filter(type="delta", value=0.2))`, "expected CE or PE"},
		{"invalid filter object", `ctx.option_chain(underlying="NIFTY", expiry="2026-01-27", option_type="CE", filter={"type":"delta","value":0.2})`, "filter must be constructed"},
		{"invalid filter type", `filter(type="unknown", value=1)`, "invalid filter type"},
		{"invalid filter value", `filter(type="delta", value="0.2")`, "value"},
		{"missing filter value", `filter(type="delta")`, "value"},
		{"missing filter type", `filter(value=0.2)`, "type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			globals := scriptBuiltins()
			f := newFixture(t)
			globals["ctx"] = &strategyContext{ctx: context.Background(), repo: f.repo, execution: &engine.Execution{Time: time.Date(2026, 1, 5, 9, 20, 0, 0, time.UTC)}}
			_, err := starlark.Eval(&starlark.Thread{Name: "test"}, "test.star", tt.expression, globals)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v; want %s", err, tt.want)
			}
		})
	}
}

func TestOptionChainOffsetReturnsSymbol(t *testing.T) {
	f := newFixture(t)
	globals := scriptBuiltins()
	globals["ctx"] = &strategyContext{ctx: context.Background(), repo: f.repo,
		execution: &engine.Execution{Time: time.Date(2026, 1, 5, 9, 25, 0, 0, time.UTC)}}
	value, err := starlark.Eval(&starlark.Thread{Name: "test"}, "test.star",
		`ctx.option_chain(underlying="NIFTY", expiry="2026-01-27", option_type="PE", filter=filter(type="offset", value=2))`, globals)
	if err != nil {
		t.Fatal(err)
	}
	if value != starlark.String("NIFTY-20260127-22650-PE") {
		t.Fatalf("unexpected symbol: %v", value)
	}
	if f.contractQueries != 1 {
		t.Fatal("offset selection must read the stored trading symbol")
	}
}
