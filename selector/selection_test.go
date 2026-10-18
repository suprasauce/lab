package selector

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

// A small database/sql test driver exercises the concrete repository without
// changing its API or adding a mocking dependency.
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
	columns []string
	values  []driver.Value
}

func (r *testRows) Columns() []string { return r.columns }
func (r *testRows) Close() error      { return nil }
func (r *testRows) Next(dest []driver.Value) error {
	if r.values == nil {
		return io.EOF
	}
	copy(dest, r.values)
	r.values = nil
	return nil
}

func TestOffsetSelectorSelectionWindow(t *testing.T) {
	input := models.SelectionInput{
		UnderlyingID: 1,
		Expiry:       time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC),
		OptionType:   models.Call,
		Timeframe:    models.Timeframe1m,
	}
	var err error
	input.EntryTime, err = time.Parse(time.RFC3339, "2024-10-14T09:20:00+05:30")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, timestamp string
		reject          bool
	}{
		{"exact target", "2024-10-14T09:20:00+05:30", false},
		{"next session", "2024-10-15T09:15:00+05:30", false},
		{"expiry day", "2024-10-31T15:30:00+05:30", false},
		{"before target", "2024-10-14T09:19:00+05:30", true},
		{"after expiry", "2024-11-01T00:00:00+05:30", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stamp, err := time.Parse(time.RFC3339, tt.timestamp)
			if err != nil {
				t.Fatal(err)
			}
			queries := 0
			db := sql.OpenDB(testConnector{query: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				queries++
				if strings.Contains(query, "FROM underlying_candles") {
					if args[0].Value != int64(1) || args[1].Value != "1m" || !args[2].Value.(time.Time).Equal(input.EntryTime) {
						t.Fatalf("unexpected candle args: %+v", args)
					}
					return &testRows{columns: []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"},
						values: []driver.Value{stamp, "1m", int64(2253000), int64(2254000), int64(2250000), int64(2251000), int64(10)}}, nil
				}
				if !strings.Contains(query, "FROM option_contracts") {
					t.Fatalf("unexpected query: %s", query)
				}
				if args[0].Value != int64(1) || !args[1].Value.(time.Time).Equal(input.Expiry) || args[2].Value != int64(2265000) || args[3].Value != "CALL" {
					t.Fatalf("unexpected contract args: %+v", args)
				}
				return &testRows{columns: []string{"id", "underlying_id", "expiry", "strike_price", "option_type"},
					values: []driver.Value{int64(7), int64(1), input.Expiry, int64(2265000), "CALL"}}, nil
			}})
			t.Cleanup(func() { db.Close() })
			repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			contract, err := NewOffsetSelector(repo, 2).Select(context.Background(), input)
			if tt.reject {
				if err == nil || queries != 1 {
					t.Fatalf("invalid candle accepted or contract queried: err=%v, queries=%d", err, queries)
				}
			} else if err != nil || queries != 2 || contract.ID != 7 || contract.StrikePrice != 2265000 {
				t.Fatalf("selection = %+v, %v; queries=%d", contract, err, queries)
			}
		})
	}
}

func TestOffsetSelectorPreservesRepositoryError(t *testing.T) {
	db := sql.OpenDB(testConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return nil, context.Canceled
	}})
	defer db.Close()
	repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := NewOffsetSelector(repo, 0).Select(context.Background(), models.SelectionInput{
		EntryTime: time.Date(2024, 10, 14, 0, 0, 0, 0, time.UTC),
		Expiry:    time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error lost cancellation: %v", err)
	}
}

func TestUnimplementedSelectors(t *testing.T) {
	for _, s := range []models.InstrumentSelector{PremiumSelector{}, DeltaSelector{}} {
		if _, err := s.Select(context.Background(), models.SelectionInput{}); err == nil || !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("expected not implemented, got %v", err)
		}
	}
}
