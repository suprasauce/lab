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
