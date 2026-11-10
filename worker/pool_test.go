package worker

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

const validScript = `
def initialize(ctx):
    ctx.configure(underlying="NIFTY", start_date="2026-01-01", end_date="2026-01-31")

def on_candle(ctx):
    if ctx.time == "09:15":
        ctx.state["entry"] = ctx.place_order(contract=offset_selector("CALL", offset=2), side="SELL", quantity=75)
    if ctx.time == "09:20":
        entry = ctx.order(ctx.state["entry"])
        if entry.status == "FILLED":
            ctx.place_order(contract=id_selector(entry.contract_id), side="BUY", quantity=75)
`

func newPoolTest(t *testing.T) (*WorkerPool, sqlmock.Sqlmock, *bytes.Buffer) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Error(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	logs := new(bytes.Buffer)
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return NewWorkerPool(repository.NewRepository(db, logger), logger), mock, logs
}

type capturedJSON struct{ value *map[string]any }

func (c capturedJSON) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	return decoder.Decode(c.value) == nil
}

func TestProcessJobStoresCompletedResults(t *testing.T) {
	pool, mock, logs := newPoolTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "script"}).AddRow(int64(1), validScript))
	mock.ExpectQuery("FROM lab.underlyings").WithArgs("NIFTY").
		WillReturnRows(sqlmock.NewRows([]string{"id", "symbol"}).AddRow(int64(7), "NIFTY"))
	mock.ExpectQuery("FROM lab.option_expiry").WithArgs(int64(7),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)).
		WillReturnRows(sqlmock.NewRows([]string{"expiry_date"}).AddRow(workerExpiry()))

	session := time.Date(2026, 1, 5, 0, 0, 0, 0, workerExpiry().Location())
	for day := workerExpiry().AddDate(0, 0, -45); !day.After(workerExpiry()); day = day.AddDate(0, 0, 1) {
		first := workerCandleRows()
		if !day.After(session) {
			first = workerCandleRows().AddRow(session.Add(9*time.Hour+15*time.Minute), "5m", 2253000, 2253000, 2253000, 2253000, 0)
		}
		mock.ExpectQuery("FROM lab.underlying_candles").WithArgs(int64(7), "5m", day.Add(9*time.Hour+15*time.Minute)).
			WillReturnRows(first)
		if !day.Equal(session) {
			continue
		}
		for tick := 1; tick <= 75; tick++ {
			start := session.Add(9*time.Hour + 15*time.Minute + time.Duration(tick-1)*5*time.Minute)
			mock.ExpectQuery("FROM lab.underlying_candles").WithArgs(int64(7), "5m", start, start.Add(time.Microsecond)).
				WillReturnRows(workerCandleRows().AddRow(start, "5m", 2253000, 2253000, 2253000, 2253000, 0))
			if tick == 1 {
				mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), workerExpiry(), int64(2265000), "CALL").
					WillReturnRows(workerContractRows())
				mock.ExpectQuery("FROM lab.option_candles").WithArgs(int64(101), "5m", start, start.Add(5*time.Minute-time.Microsecond)).
					WillReturnRows(workerCandleRows().AddRow(start, "5m", 10000, 10000, 10000, 10000, 0))
			}
			if tick == 2 {
				mock.ExpectQuery("FROM lab.option_candles").WithArgs(int64(101), "5m", start, start).
					WillReturnRows(workerCandleRows().AddRow(start, "5m", 8000, 8000, 8000, 8000, 0))
				mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(101)).WillReturnRows(workerContractRows())
			}
		}
	}
	var payload map[string]any
	mock.ExpectExec(regexp.QuoteMeta(repository.QueryUpdateBacktest)).
		WithArgs(int64(1), string(models.BacktestCompleted), capturedJSON{&payload}).
		WillReturnResult(sqlmock.NewResult(0, 1))

	pool.processJob(1)

	if payload == nil {
		t.Fatalf("completed results were not stored: %s", logs)
	}
	summary := payload["summary"].(map[string]any)
	if summary["total_trades"] != json.Number("1") || summary["total_pnl"] != json.Number("1500.00") {
		t.Fatalf("incorrect summary: %v", summary)
	}
	trades := payload["trades"].([]any)
	if len(trades) != 1 {
		t.Fatalf("incorrect trade count: %d", len(trades))
	}
	trade := trades[0].(map[string]any)
	if len(trade) != 11 || trade["strike"] != json.Number("22650.00") || trade["entry_price"] != json.Number("100.00") || trade["exit_price"] != json.Number("80.00") || trade["pnl"] != json.Number("1500.00") || trade["side"] != "SHORT" || trade["entry_timestamp"] != "2026-01-05T09:15:00+05:30" || trade["exit_timestamp"] != "2026-01-05T09:20:00+05:30" {
		t.Fatalf("incorrect persisted trade: %v", trade)
	}
}

func TestProcessJobInvalidScriptFails(t *testing.T) {
	pool, mock, _ := newPoolTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "script"}).AddRow(int64(1), "def broken("))
	var payload map[string]any
	mock.ExpectExec(regexp.QuoteMeta(repository.QueryUpdateBacktest)).
		WithArgs(int64(1), string(models.BacktestFailed), capturedJSON{&payload}).
		WillReturnResult(sqlmock.NewResult(0, 1))

	pool.processJob(1)

	if len(payload) != 1 || !strings.Contains(payload["error"].(string), "load strategy") {
		t.Fatalf("failure details not stored: %v", payload)
	}
}

func TestProcessJobExecutionErrorFails(t *testing.T) {
	pool, mock, _ := newPoolTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "script"}).AddRow(int64(1), validScript))
	mock.ExpectQuery("FROM lab.underlyings").WithArgs("NIFTY").WillReturnError(errors.New("market data unavailable"))
	var payload map[string]any
	mock.ExpectExec(regexp.QuoteMeta(repository.QueryUpdateBacktest)).
		WithArgs(int64(1), string(models.BacktestFailed), capturedJSON{&payload}).
		WillReturnResult(sqlmock.NewResult(0, 1))

	pool.processJob(1)

	if len(payload) != 1 || !strings.Contains(payload["error"].(string), "market data unavailable") {
		t.Fatalf("execution failure details not stored: %v", payload)
	}
}

func TestProcessJobUpdateFailureIsLogged(t *testing.T) {
	pool, mock, logs := newPoolTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "script"}).AddRow(int64(1), "invalid script"))
	mock.ExpectExec(regexp.QuoteMeta(repository.QueryUpdateBacktest)).
		WithArgs(int64(1), string(models.BacktestFailed), sqlmock.AnyArg()).WillReturnError(errors.New("database unavailable"))

	pool.processJob(1)

	if !strings.Contains(logs.String(), "failed to persist backtest result") || !strings.Contains(logs.String(), `"backtest_id":1`) {
		t.Fatalf("persistence failure was not logged: %s", logs)
	}
}

func TestProcessJobDeletedBacktestUpdateIsLogged(t *testing.T) {
	pool, mock, logs := newPoolTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "script"}).AddRow(int64(1), "invalid script"))
	mock.ExpectExec(regexp.QuoteMeta(repository.QueryUpdateBacktest)).
		WithArgs(int64(1), string(models.BacktestFailed), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))

	pool.processJob(1)

	if !strings.Contains(logs.String(), "failed to persist backtest result") {
		t.Fatal("zero affected rows were treated as a successful update")
	}
}

func TestWorkerContinuesAfterFetchFailure(t *testing.T) {
	pool, mock, _ := newPoolTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryPendingBacktestIDs)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(int64(1)).WillReturnError(errors.New("fetch failed"))
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "script"}).AddRow(int64(2), "invalid script"))
	mock.ExpectExec(regexp.QuoteMeta(repository.QueryUpdateBacktest)).
		WithArgs(int64(2), string(models.BacktestFailed), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	pool.SubmitJob(1)
	pool.SubmitJob(2)
	close(pool.jobs)
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if mock.ExpectationsWereMet() == nil {
			return
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("worker did not process the next job")
		}
	}
}

func TestWorkerQueuesPendingBacktestsOnStartup(t *testing.T) {
	pool, mock, _ := newPoolTest(t)
	rows := sqlmock.NewRows([]string{"id"})
	// Recovery must also work when the backlog exceeds the channel capacity.
	for id := int64(1); id <= queueCapacity+2; id++ {
		rows.AddRow(id)
	}
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryPendingBacktestIDs)).WillReturnRows(rows).RowsWillBeClosed()
	for id := int64(1); id <= queueCapacity+2; id++ {
		mock.ExpectQuery(regexp.QuoteMeta(repository.QueryGetBacktest)).WithArgs(id).
			WillReturnRows(sqlmock.NewRows([]string{"id", "script"}).AddRow(id, "invalid script"))
		mock.ExpectExec(regexp.QuoteMeta(repository.QueryUpdateBacktest)).
			WithArgs(id, string(models.BacktestFailed), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	started := make(chan error, 1)
	go func() { started <- pool.Start() }()
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("startup blocked while recovering pending jobs")
	}
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	close(pool.jobs)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if mock.ExpectationsWereMet() == nil {
			return
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("worker did not process recovered jobs")
		}
	}
}

func TestWorkerStartupReturnsPendingQueryError(t *testing.T) {
	pool, mock, _ := newPoolTest(t)
	failure := errors.New("database unavailable")
	mock.ExpectQuery(regexp.QuoteMeta(repository.QueryPendingBacktestIDs)).WillReturnError(failure)
	if err := pool.Start(); !errors.Is(err, failure) {
		t.Fatalf("expected startup failure, got %v", err)
	}
	if err := pool.Start(); !errors.Is(err, failure) {
		t.Fatalf("expected same failure from repeated Start, got %v", err)
	}
}

func workerExpiry() time.Time {
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 1, 27, 0, 0, 0, 0, location)
}

func workerCandleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
}

func workerContractRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}).
		AddRow(int64(101), int64(7), int64(9), workerExpiry(), int64(2265000), "CALL")
}
