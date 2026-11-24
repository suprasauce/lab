package scripting

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"io"
	"log/slog"
	"optionsbacktest/repository"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"optionsbacktest/chart"
	"optionsbacktest/engine"
)

func TestChartEMA(t *testing.T) {
	for _, tt := range []struct{ name, expression, want string }{
		{"keyword arguments", `chart.ema(tradingsymbol="NIFTY", period=20, timeframe="5m")`, ""},
		{"positional arguments", `chart.ema("NIFTY", 20, "5m")`, ""},
		{"missing argument", `chart.ema("NIFTY", 20)`, "missing"},
		{"wrong symbol type", `chart.ema(1, 20, "5m")`, "want string"},
		{"empty symbol", `chart.ema(" ", 20, "5m")`, "tradingsymbol must be nonempty"},
		{"zero period", `chart.ema("NIFTY", 0, "5m")`, "period must be positive"},
		{"negative period", `chart.ema("NIFTY", -1, "5m")`, "period must be positive"},
		{"fractional period", `chart.ema("NIFTY", 2.5, "5m")`, "want int"},
		{"unsupported timeframe", `chart.ema("NIFTY", 20, "2m")`, "unsupported timeframe"},
		{"one-minute timeframe", `chart.ema("NIFTY", 20, "1m")`, "expected 5m"},
		{"fifteen-minute timeframe", `chart.ema("NIFTY", 20, "15m")`, "expected 5m"},
		{"daily timeframe", `chart.ema("NIFTY", 20, "1d")`, "expected 5m"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if tt.want == "" {
				mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}))
			}
			thread := &starlark.Thread{Name: "chart-test"}
			thread.SetLocal("strategy_context", &strategyContext{
				ctx:       context.Background(),
				execution: &engine.Execution{Time: time.Date(2026, 1, 1, 9, 25, 0, 0, time.UTC), ChartService: chart.NewChartService(repo)},
			})
			value, err := starlark.Eval(thread, "chart.star", tt.expression, scriptBuiltins())
			if tt.want == "" {
				if err != nil || value != starlark.None {
					t.Fatalf("empty history: %v, %v", value, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v; want %q", err, tt.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChartEMARequiresSimulation(t *testing.T) {
	_, err := starlark.Eval(&starlark.Thread{Name: "chart-test"}, "chart.star", `chart.ema("NIFTY", 20, "5m")`, scriptBuiltins())
	if err == nil || !strings.Contains(err.Error(), "only available during on_candle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChartEMAUsesCallbackContext(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	at := time.Date(2026, 1, 5, 9, 15, 0, 0, time.FixedZone("IST", 19800))
	columns := []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}
	mock.ExpectQuery("FROM lab.underlying_candles").WillReturnRows(sqlmock.NewRows(columns).AddRow(at, "5m", 10000, 10000, 10000, 10000, 0))
	mock.ExpectQuery("SELECT c.start_time").WithArgs("NIFTY").WillReturnRows(sqlmock.NewRows(columns).AddRow(at, "5m", 10000, 10000, 10000, 10000, 0))
	script, err := LoadSource("chart.star", `
def initialize(ctx):
    ctx.configure()
    ctx.simulate(start_date="2026-01-05", end_date="2026-01-05")

def on_candle(ctx):
    value = chart.ema("NIFTY", 9, "5m")
    if ctx.time < "09:20":
        if value != None:
            fail("unfinished candle exposed")
    elif value != 100.0:
        fail("expected EMA in rupees")
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := script.Run(context.Background(), engine.NewEngine(repo)); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
