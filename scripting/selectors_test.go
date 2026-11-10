package scripting

import (
	"testing"

	"go.starlark.net/starlark"

	"optionsbacktest/models"
	"optionsbacktest/selector"
)

func evaluateSelector(t *testing.T, expression string) *selectorValue {
	t.Helper()
	value, err := starlark.Eval(&starlark.Thread{Name: "selector-test"}, "selector.star", expression, selectorBuiltins())
	if err != nil {
		t.Fatal(err)
	}
	wrapped, ok := value.(*selectorValue)
	if !ok {
		t.Fatalf("constructor returned %T instead of selectorValue", value)
	}
	return wrapped
}

func TestOffsetSelectorBuiltinParsesArguments(t *testing.T) {
	value := evaluateSelector(t, `offset_selector("PUT", offset=-2)`)
	selection, ok := value.selector.(*selector.OffsetSelector)
	if !ok || selection.OptionType != models.Put || selection.StrikeOffset != -2 || value.Type() != "offset_selector" {
		t.Fatalf("incorrect offset selector: %+v", value)
	}
}

func TestOffsetSelectorBuiltinDefaultsToATM(t *testing.T) {
	value := evaluateSelector(t, `offset_selector("CALL")`)
	selection, ok := value.selector.(*selector.OffsetSelector)
	if !ok || selection.OptionType != models.Call || selection.StrikeOffset != 0 {
		t.Fatalf("incorrect default selector: %+v", value)
	}
}

func TestIDSelectorBuiltinParsesArguments(t *testing.T) {
	value := evaluateSelector(t, `id_selector(id=101)`)
	selection, ok := value.selector.(*selector.IDSelector)
	if !ok || selection.ContractID != 101 || value.Type() != "id_selector" {
		t.Fatalf("incorrect ID selector: %+v", value)
	}
}

func TestWrappedSelectorPreservesStarlarkBehavior(t *testing.T) {
	selection := selector.NewIDSelector(101)
	value := wrapSelector("id_selector", selection)
	wrapped, ok := value.(*selectorValue)
	if !ok || wrapped.selector != selection {
		t.Fatal("wrapper replaced the selector")
	}
	value.Freeze()
	if value.String() != "id_selector(...)" || value.Type() != "id_selector" || value.Truth() != starlark.True {
		t.Fatal("selector value behavior changed")
	}
	if _, err := value.Hash(); err == nil {
		t.Fatal("selector must remain unhashable")
	}
}
