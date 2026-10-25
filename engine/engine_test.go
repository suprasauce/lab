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

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

type forbiddenSelector struct{ calls int }

func (s *forbiddenSelector) SelectStrike(models.Candle) (models.Price, error) {
	s.calls++
	return 0, errors.New("leg processing must not run")
}

func TestExpirySimulationClock(t *testing.T) {
	for _, tt := range []struct {
		name   string
		expiry time.Time
		dtes   []int
		dates  []string
	}{
		{"maximum DTE and leap day", time.Date(2024, 3, 1, 18, 0, 0, 0, time.UTC), []int{1, 3, 2}, []string{"2024-02-27", "2024-02-28", "2024-02-29", "2024-03-01"}},
		{"expiry day only", time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), []int{0}, []string{"2026-09-29"}},
		{"year boundary", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), []int{2, 0}, []string{"2025-12-30", "2025-12-31", "2026-01-01"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selector := &forbiddenSelector{}
			strategy := models.Strategy{UnderlyingID: 7}
			for _, dte := range tt.dtes {
				strategy.Legs = append(strategy.Legs, models.OptionLeg{EntryDTE: dte, StrikeSelector: selector})
			}
			queries := 0
			db := sql.OpenDB(testConnector{query: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				if queries >= len(tt.dates) {
					t.Fatal("simulation passed expiry day")
				}
				if !strings.Contains(query, "FROM lab.underlying_candles") || !strings.Contains(query, "start_time >= $3") || !strings.Contains(query, "start_time < $4") || !strings.Contains(query, "ORDER BY start_time ASC") {
					t.Fatalf("unexpected simulation query: %s", query)
				}
				start := args[2].Value.(time.Time)
				end := args[3].Value.(time.Time)
				want, err := time.ParseInLocation(time.DateOnly, tt.dates[queries], marketLocation)
				if err != nil {
					t.Fatal(err)
				}
				if args[0].Value != int64(7) || args[1].Value != "5m" || !start.Equal(want) || !end.Equal(want.AddDate(0, 0, 1)) {
					t.Fatalf("unexpected daily bounds: %+v; want %s", args, want)
				}
				queries++
				rows := &testRows{columns: []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}}
				// Include an empty calendar day and multiple timestamps on others.
				if queries != 2 {
					for _, minute := range []int{15, 20} {
						rows.records = append(rows.records, []driver.Value{want.Add(9*time.Hour + time.Duration(minute)*time.Minute), "5m", int64(100), int64(110), int64(90), int64(105), int64(0)})
					}
				}
				return rows, nil
			}})
			defer db.Close()
			repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := NewEngine(repo).backtestExpiry(context.Background(), &strategy, tt.expiry); err != nil {
				t.Fatal(err)
			}
			if queries != len(tt.dates) || selector.calls != 0 {
				t.Fatalf("queries=%d selector calls=%d", queries, selector.calls)
			}
		})
	}
}

func TestExpirySimulationStopsOnQueryFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	queries := 0
	db := sql.OpenDB(testConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		queries++
		return nil, failure
	}})
	defer db.Close()
	repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	strategy := models.Strategy{UnderlyingID: 7, Legs: []models.OptionLeg{{EntryDTE: 2}}}
	err := NewEngine(repo).backtestExpiry(context.Background(), &strategy, time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "2026-09-27") || !strings.Contains(err.Error(), "2026-09-29") || queries != 1 {
		t.Fatalf("unexpected fail-fast result: err=%v queries=%d", err, queries)
	}
}

func TestUnderlyingCandleRangeResultsAndErrors(t *testing.T) {
	failure := errors.New("iteration failed")
	for _, name := range []string{"empty", "candles", "scan error", "iteration error"} {
		t.Run(name, func(t *testing.T) {
			start := time.Date(2026, 9, 1, 0, 0, 0, 0, marketLocation)
			rows := &testRows{columns: []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}}
			switch name {
			case "candles":
				for _, minute := range []int{15, 20} {
					rows.records = append(rows.records, []driver.Value{start.Add(9*time.Hour + time.Duration(minute)*time.Minute), "5m", int64(100), int64(110), int64(90), int64(105), int64(0)})
				}
			case "scan error":
				rows.values = []driver.Value{start, "5m", "invalid price", int64(110), int64(90), int64(105), int64(0)}
			case "iteration error":
				rows.iterationErr = failure
			}
			db := sql.OpenDB(testConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) { return rows, nil }})
			defer db.Close()
			repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			candles, err := repo.GetUnderlyingCandles(context.Background(), 7, start, start.AddDate(0, 0, 1), models.Timeframe5m)
			switch name {
			case "empty":
				if err != nil || candles == nil || len(candles) != 0 {
					t.Fatalf("expected non-nil empty slice, got %+v %v", candles, err)
				}
			case "candles":
				if err != nil || len(candles) != 2 || !candles[0].Timestamp.Before(candles[1].Timestamp) || candles[0].Open != 100 {
					t.Fatalf("unexpected candles: %+v %v", candles, err)
				}
			case "scan error":
				if err == nil || !strings.Contains(err.Error(), "scan underlying candle") {
					t.Fatalf("expected scan error, got %v", err)
				}
			case "iteration error":
				if !errors.Is(err, failure) {
					t.Fatalf("iteration cause lost: %v", err)
				}
			}
			if !rows.closed {
				t.Fatal("rows were not closed")
			}
		})
	}
}

type queryFunc func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
type testConnector struct{ query queryFunc }
type testConn struct{ query queryFunc }

func (c testConnector) Connect(context.Context) (driver.Conn, error) { return testConn{c.query}, nil }
func (c testConnector) Driver() driver.Driver                        { return testDriver{} }

type testDriver struct{}

func (testDriver) Open(string) (driver.Conn, error)  { return nil, errors.New("use connector") }
func (testConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (testConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }
func (testConn) Close() error                        { return nil }
func (c testConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type testRows struct {
	columns      []string
	values       []driver.Value
	records      [][]driver.Value
	iterationErr error
	closed       bool
}

func (r *testRows) Columns() []string { return r.columns }
func (r *testRows) Close() error      { r.closed = true; return nil }
func (r *testRows) Next(dest []driver.Value) error {
	if len(r.records) > 0 {
		copy(dest, r.records[0])
		r.records = r.records[1:]
		return nil
	}
	if r.values == nil {
		if r.iterationErr != nil {
			return r.iterationErr
		}
		return io.EOF
	}
	copy(dest, r.values)
	r.values = nil
	return nil
}
