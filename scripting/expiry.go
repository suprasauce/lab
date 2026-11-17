package scripting

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
)

// expiryValue is an immutable calendar expiry returned by get_expiries.
type expiryValue struct {
	date       time.Time
	underlying string
}

func (e *expiryValue) String() string {
	return fmt.Sprintf("expiry(%s, %s)", e.underlying, e.date.Format(time.DateOnly))
}
func (*expiryValue) Type() string          { return "expiry" }
func (*expiryValue) Freeze()               {}
func (*expiryValue) Truth() starlark.Bool  { return starlark.True }
func (*expiryValue) Hash() (uint32, error) { return 0, fmt.Errorf("expiry is unhashable") }
func (*expiryValue) AttrNames() []string   { return []string{"date", "underlying", "get_dte"} }
func (e *expiryValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "date":
		return starlark.String(e.date.Format(time.DateOnly)), nil
	case "underlying":
		return starlark.String(e.underlying), nil
	case "get_dte":
		return starlark.NewBuiltin(name, e.getDTE), nil
	}
	return nil, nil
}

func (e *expiryValue) getDTE(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var dateText string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "date", &dateText); err != nil {
		return nil, err
	}
	date, err := time.Parse(time.DateOnly, dateText)
	if err != nil {
		return nil, fmt.Errorf("get_dte date: %w", err)
	}
	// Parse both calendar dates in UTC so clock components and DST cannot affect DTE.
	expiry, err := time.Parse(time.DateOnly, e.date.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	return starlark.MakeInt(int(expiry.Sub(date) / (24 * time.Hour))), nil
}
