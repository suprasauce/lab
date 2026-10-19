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

type recordingSelector struct {
	input models.SelectionInput
	err   error
}

func (s *recordingSelector) Select(ctx context.Context, input models.SelectionInput) (models.Option, error) {
	s.input = input
	return models.Option{ID: 10, StrikePrice: 22500 * 100, OptionType: input.OptionType}, s.err
}

func TestPendingLegEntryTarget(t *testing.T) {
	tests := []struct {
		name, expiry, want string
		dte, hour, minute  int
	}{
		{"calendar days include weekends", "2024-10-31", "2024-09-16T09:20:00+05:30", 45, 9, 20},
		{"expiry day", "2024-10-31", "2024-10-31T15:00:00+05:30", 0, 15, 0},
		{"leap day", "2024-03-01", "2024-02-29T09:20:00+05:30", 1, 9, 20},
		{"year boundary", "2025-01-01", "2024-12-31T00:00:00+05:30", 1, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expiry, err := time.Parse(time.DateOnly, tt.expiry)
			if err != nil {
				t.Fatal(err)
			}
			selector := &recordingSelector{}
			execution := models.LegExecution{
				Leg: &models.OptionLeg{EntryDTE: tt.dte, EntryHour: tt.hour, EntryMinute: tt.minute,
					OptionType: models.Call, InstrumentSelector: selector},
				State: models.LegState{Status: models.LegPending},
			}
			queries := 0
			db := sql.OpenDB(testConnector{query: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				queries++
				wantEntry, err := time.Parse(time.RFC3339, tt.want)
				if err != nil {
					t.Fatal(err)
				}
				wantExit := time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 15, 30, 0, 0, marketLocation)
				if !strings.Contains(query, "FROM option_candles") || len(args) != 4 || args[0].Value != int64(10) || args[1].Value != "1m" || !args[2].Value.(time.Time).Equal(wantEntry) || !args[3].Value.(time.Time).Equal(wantExit) {
					t.Fatalf("unexpected candle request: %s, %+v", query, args)
				}
				return &testRows{columns: []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}}, nil
			}})
			t.Cleanup(func() { db.Close() })
			repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := NewEngine(repo).backtestOptionLeg(context.Background(), execution.Leg.(*models.OptionLeg), &execution.State, expiry, 1); err != nil {
				t.Fatal(err)
			}
			if queries != 1 {
				t.Fatalf("expected one candle query, got %d", queries)
			}
			if got := execution.State.EntryTargetTime.Format(time.RFC3339); got != tt.want {
				t.Fatalf("target = %s; want %s", got, tt.want)
			}
			if selector.input.UnderlyingID != 1 || !selector.input.Expiry.Equal(expiry) ||
				!selector.input.EntryTime.Equal(execution.State.EntryTargetTime) || selector.input.Timeframe != models.Timeframe1m || selector.input.OptionType != models.Call {
				t.Fatalf("unexpected selection input: %+v", selector.input)
			}
			if execution.State.Status != models.LegClosed || execution.State.SelectedContract == nil || execution.State.SelectedContract.ID != 10 {
				t.Fatalf("unexpected runtime state: %+v", execution.State)
			}
		})
	}
}

func TestPendingLegSelectorError(t *testing.T) {
	failure := errors.New("selection failed")
	leg := &models.OptionLeg{
		EntryDTE: 1, EntryHour: 9, EntryMinute: 20,
		OptionType: models.Put, InstrumentSelector: &recordingSelector{err: failure},
	}
	state := models.LegState{Status: models.LegPending}
	err := new(Engine).backtestOptionLeg(context.Background(), leg, &state, time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC), 1)
	if !errors.Is(err, failure) {
		t.Fatalf("error lost selection cause: %v", err)
	}
	if state.SelectedContract != nil || state.Status != models.LegPending {
		t.Fatal("failure changed execution state")
	}
}

func TestOptionsExpiryRejectsOtherLegTypes(t *testing.T) {
	expiry := time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC)
	var nilOption *models.OptionLeg
	for _, leg := range []models.Leg{&models.FutureLeg{}, &models.EquityLeg{}, nil, nilOption} {
		strategy := &models.Strategy{Legs: []models.Leg{leg}}
		err := new(Engine).backtestExpiry(context.Background(), strategy, expiry)
		if err == nil || !strings.Contains(err.Error(), "2024-10-31") || !strings.Contains(err.Error(), "leg 0") {
			t.Fatalf("expected dated leg error for %T, got %v", leg, err)
		}
	}
}

func TestOptionsExpiryPropagatesLegFailure(t *testing.T) {
	failure := errors.New("selection failed")
	strategy := &models.Strategy{UnderlyingID: 1, Legs: []models.Leg{
		&models.OptionLeg{EntryHour: 9, EntryMinute: 20, OptionType: models.Call, InstrumentSelector: &recordingSelector{err: failure}},
		&models.EquityLeg{}, // Must not execute after the first leg fails.
	}}
	err := new(Engine).backtestExpiry(context.Background(), strategy, time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "leg 0") {
		t.Fatalf("expected first leg selection failure, got %v", err)
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

func TestOpenLegCandleQueryError(t *testing.T) {
	failure := errors.New("candle query failed")
	db := sql.OpenDB(testConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
		return nil, failure
	}})
	defer db.Close()
	repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	expiry := time.Date(2024, 10, 31, 0, 0, 0, 0, time.UTC)
	state := models.LegState{Status: models.LegOpen, EntryTargetTime: expiry, SelectedContract: &models.Option{ID: 10}}
	err := NewEngine(repo).backtestOptionLeg(context.Background(), &models.OptionLeg{}, &state, expiry, 1)
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "fetch option candles") {
		t.Fatalf("expected wrapped fetch error, got %v", err)
	}
	if state.Status != models.LegOpen {
		t.Fatalf("status changed to %s", state.Status)
	}
}
