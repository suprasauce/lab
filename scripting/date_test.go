package scripting

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestDateAdd(t *testing.T) {
	for _, tt := range []struct {
		date string
		days int
		want string
	}{
		{"2026-01-27", -45, "2025-12-13"},
		{"2024-02-28", 1, "2024-02-29"},
		{"2026-01-27", 0, "2026-01-27"},
	} {
		value, err := addDate(nil, starlark.NewBuiltin("date.add", addDate), starlark.Tuple{starlark.String(tt.date), starlark.MakeInt(tt.days)}, nil)
		if err != nil || value != starlark.String(tt.want) {
			t.Fatalf("date.add(%s, %d) = %v, %v; want %s", tt.date, tt.days, value, err, tt.want)
		}
	}
}
