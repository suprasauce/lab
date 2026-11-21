package scripting

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
)

// addDate shifts a YYYY-MM-DD calendar date by the given number of days.
func addDate(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var text string
	var days int
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "date", &text, "days", &days); err != nil {
		return nil, err
	}
	date, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return nil, fmt.Errorf("date.add: %w", err)
	}
	return starlark.String(date.AddDate(0, 0, days).Format(time.DateOnly)), nil
}
