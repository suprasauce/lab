package scripting

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.starlark.net/starlark"
	"optionsbacktest/engine"
	"optionsbacktest/repository"
)

func TestGetExpiry(t *testing.T) {
	for _, tt := range []struct {
		name, expression, date, expiry, wantError string
		expiryType                                string
		offset                                    int
		missing, databaseError, initializing      bool
	}{
		{name: "nearest expiry after current month expired", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 0)`, date: "2026-01-31", expiry: "2026-02-24"},
		{name: "next month from January 31", expression: `ctx.get_expiry(underlying="NIFTY", expiry_type="monthly", offset=1)`, date: "2026-01-31", expiry: "2026-03-30", offset: 1},
		{name: "previous expiry from February base", expression: `ctx.get_expiry("NIFTY", "MONTHLY", -1)`, date: "2026-01-31", expiry: "2026-01-27", offset: -1},
		{name: "before expiry", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 0)`, date: "2026-01-05", expiry: "2026-01-27"},
		{name: "on expiry", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 0)`, date: "2026-01-27", expiry: "2026-01-27"},
		{name: "negative offset crosses year", expression: `ctx.get_expiry("NIFTY", "MONTHLY", -1)`, date: "2026-01-05", expiry: "2025-12-30", offset: -1},
		{name: "unavailable offset", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 100)`, date: "2026-01-31", offset: 100, missing: true},
		{name: "missing", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 0)`, date: "2026-01-31", missing: true},
		{name: "database error", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 0)`, date: "2026-01-31", databaseError: true, wantError: "database unavailable"},
		{name: "weekly", expression: `ctx.get_expiry("NIFTY", "WEEKLY", 0)`, expiryType: "WEEKLY", date: "2026-01-05", expiry: "2026-01-06"},
		{name: "monthly counts as weekly", expression: `ctx.get_expiry("NIFTY", "WEEKLY", 0)`, expiryType: "WEEKLY", date: "2026-01-26", expiry: "2026-01-27"},
		{name: "unsupported expiry type", expression: `ctx.get_expiry("NIFTY", "DAILY", 0)`, wantError: "not supported"},
		{name: "fractional offset", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 0.5)`, wantError: "want int"},
		{name: "missing offset", expression: `ctx.get_expiry("NIFTY", "MONTHLY")`, wantError: "missing"},
		{name: "empty underlying", expression: `ctx.get_expiry("", "MONTHLY", 0)`, wantError: "nonempty"},
		{name: "initialization", expression: `ctx.get_expiry("NIFTY", "MONTHLY", 0)`, initializing: true, wantError: "only available during on_candle"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			at := time.Date(2026, 1, 31, 9, 30, 0, 0, time.FixedZone("IST", 19800))
			if tt.date != "" {
				at, _ = time.ParseInLocation(time.DateOnly, tt.date, at.Location())
				kind := tt.expiryType
				if kind == "" {
					kind = "MONTHLY"
				}
				query := mock.ExpectQuery("WITH expiries AS").WithArgs("NIFTY", kind, tt.date, tt.offset)
				if tt.databaseError {
					query.WillReturnError(errors.New("database unavailable"))
				} else {
					rows := sqlmock.NewRows([]string{"expiry_date"})
					if !tt.missing {
						expiry, _ := time.ParseInLocation(time.DateOnly, tt.expiry, at.Location())
						rows.AddRow(expiry)
					}
					query.WillReturnRows(rows)
				}
			}
			c := &strategyContext{ctx: context.Background(), repo: repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))), execution: &engine.Execution{Time: at}, initializing: tt.initializing}
			value, err := starlark.Eval(&starlark.Thread{}, "expiry.star", tt.expression, starlark.StringDict{"ctx": c})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error=%v; want %s", err, tt.wantError)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if tt.missing {
				if value != starlark.None {
					t.Fatalf("expected None, got %v", value)
				}
			} else {
				expiry, ok := value.(*expiryValue)
				if !ok || expiry.date.Format(time.DateOnly) != tt.expiry || expiry.underlying != "NIFTY" {
					t.Fatalf("unexpected expiry: %v", value)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
