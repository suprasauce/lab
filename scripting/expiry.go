package scripting

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.starlark.net/starlark"
)

// expiryValue is an immutable calendar expiry returned by expiry discovery.
type expiryValue struct {
	date       time.Time
	underlying string
}

func (c *strategyContext) getExpiry(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var underlying, expiryType string
	var offset int
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "underlying", &underlying, "expiry_type", &expiryType, "offset", &offset); err != nil {
		return nil, err
	}
	if strings.TrimSpace(underlying) == "" {
		return nil, fmt.Errorf("underlying must be nonempty")
	}
	expiryType = strings.ToUpper(expiryType)
	if expiryType != "MONTHLY" {
		return nil, fmt.Errorf("expiry_type %q is not supported: expected MONTHLY", expiryType)
	}
	if c.initializing || c.execution == nil {
		return nil, fmt.Errorf("get_expiry is only available during on_candle")
	}
	at := c.execution.Time
	month := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, at.Location()).AddDate(0, offset, 0)
	expiry, err := c.repo.GetExpiry(c.ctx, underlying, expiryType, month)
	if errors.Is(err, sql.ErrNoRows) {
		return starlark.None, nil
	}
	if err != nil {
		return nil, err
	}
	return &expiryValue{date: expiry, underlying: underlying}, nil
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
