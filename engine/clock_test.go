package engine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"optionsbacktest/repository"
)

func TestExpirySimulationClock(t *testing.T) {
	for _, dates := range [][]string{
		{"2024-02-27", "2024-02-28", "2024-02-29", "2024-03-01"},
		{"2026-09-29"},
		{"2025-12-30", "2025-12-31", "2026-01-01"},
	} {
		t.Run(dates[0], func(t *testing.T) {
			queries := 0
			db := sql.OpenDB(testConnector{query: func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				if queries >= len(dates) || !strings.Contains(query, "LIMIT 1") || len(args) != 3 {
					t.Fatalf("unexpected session probe: %s %+v", query, args)
				}
				day, _ := time.ParseInLocation(time.DateOnly, dates[queries], marketLocation)
				start := day.Add(9*time.Hour + 15*time.Minute)
				if args[0].Value != int64(7) || args[1].Value != "5m" || !args[2].Value.(time.Time).Equal(start) {
					t.Fatalf("unexpected session parameters: %+v", args)
				}
				queries++
				rows := &testRows{columns: []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}}
				if queries == 2 {
					// No session today; even a future candle must not activate it.
					start = start.AddDate(0, 0, 1)
				}
				rows.values = []driver.Value{start, "5m", int64(100), int64(110), int64(90), int64(105), int64(0)}
				return rows, nil
			}})
			defer db.Close()
			repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			start, _ := time.Parse(time.DateOnly, dates[0])
			expiry, _ := time.Parse(time.DateOnly, dates[len(dates)-1])
			var ticks []time.Time
			err := NewEngine(repo).backtestExpiry(context.Background(), 7, expiry, start, func(at time.Time) error {
				ticks = append(ticks, at)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			sessions := len(dates)
			if sessions > 1 {
				sessions--
			}
			if queries != len(dates) || len(ticks) != sessions*76 {
				t.Fatalf("queries=%d ticks=%d; want %d sessions of 76 ticks", queries, len(ticks), sessions)
			}
			for i, at := range ticks {
				if i%76 == 0 && (at.Hour() != 9 || at.Minute() != 15) {
					t.Fatalf("bad session start: %s", at)
				}
				if i%76 == 75 && (at.Hour() != 15 || at.Minute() != 30) {
					t.Fatalf("bad session end: %s", at)
				}
				if i%76 != 0 && at.Sub(ticks[i-1]) != 5*time.Minute {
					t.Fatalf("clock skipped missing bars: %s", at)
				}
			}
		})
	}
}

func TestExpirySimulationStopsOnQueryFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	db := sql.OpenDB(testConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) { return nil, failure }})
	defer db.Close()
	repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	err := NewEngine(repo).backtestExpiry(context.Background(), 7, start.AddDate(0, 0, 2), start, func(time.Time) error { return nil })
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "2026-09-27") || !strings.Contains(err.Error(), "2026-09-29") {
		t.Fatalf("unexpected error: %v", err)
	}
}
