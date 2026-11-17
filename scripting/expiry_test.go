package scripting

import (
	"testing"
	"time"

	"go.starlark.net/starlark"
	"optionsbacktest/engine"
)

func TestExpiryCalendarDTE(t *testing.T) {
	for _, tt := range []struct {
		expiry, date string
		want         int
	}{
		{"2026-01-27", "2025-12-13", 45},
		{"2024-03-01", "2024-02-28", 2},
		{"2026-01-27", "2026-01-27", 0},
		{"2026-01-27", "2026-01-28", -1},
	} {
		t.Run(tt.date+"/"+tt.expiry, func(t *testing.T) {
			date, _ := time.Parse(time.DateOnly, tt.expiry)
			expiry := &expiryValue{date: date, underlying: "NIFTY"}
			got, err := starlark.Eval(&starlark.Thread{}, "test.star", `expiry.get_dte(date)`, starlark.StringDict{"expiry": expiry, "date": starlark.String(tt.date)})
			if err != nil {
				t.Fatal(err)
			}
			value, err := starlark.AsInt32(got)
			if err != nil || value != tt.want {
				t.Fatalf("dte=%v error=%v; want %d", got, err, tt.want)
			}
		})
	}
}

func TestExpiryInvalidDate(t *testing.T) {
	expiry := &expiryValue{}
	_, err := starlark.Eval(&starlark.Thread{}, "test.star", `expiry.get_dte("invalid")`, starlark.StringDict{"expiry": expiry})
	if err == nil {
		t.Fatal("expected date parsing error")
	}
}

func TestContextDateAndTime(t *testing.T) {
	c := &strategyContext{}
	date, _ := c.Attr("date")
	if date != starlark.None {
		t.Fatal("date available outside simulation")
	}
	c.execution = &engine.Execution{Time: time.Date(2026, 1, 15, 9, 30, 0, 0, time.FixedZone("IST", 19800))}
	date, _ = c.Attr("date")
	clock, _ := c.Attr("time")
	if date != starlark.String("2026-01-15") || clock != starlark.String("09:30") {
		t.Fatalf("date=%v time=%v", date, clock)
	}
	if old, _ := c.Attr("dte"); old != nil {
		t.Fatal("ctx.dte still exposed")
	}
}
