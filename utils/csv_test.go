package utils

import (
	"bytes"
	"encoding/csv"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/models"
)

func csvResult() *engine.BacktestResult {
	contract := &models.Option{ID: 101, StrikePrice: 2500005, OptionType: models.Call, Expiry: time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)}
	return &engine.BacktestResult{Underlying: "NIFTY", Executions: [][]*engine.Leg{{{
		Contract: contract,
		LegExecution: models.LegExecution{Trades: []models.Trade{
			{ContractID: 101, Side: models.Short, Quantity: 75,
				EntryTime: time.Date(2026, 1, 1, 3, 45, 0, 0, time.UTC), EntryPrice: 12705,
				ExitTime: time.Date(2026, 1, 6, 3, 50, 0, 0, time.UTC), ExitPrice: 8010,
				Status: models.TradeClosed, PnL: 352125},
			{ContractID: 101, Side: models.Long, Quantity: 1,
				EntryTime: time.Date(2026, 1, 7, 3, 45, 0, 0, time.UTC), EntryPrice: 29,
				Status: models.TradeOpen},
		}},
	}}}}
}

func TestWriteBacktestCSV(t *testing.T) {
	var output bytes.Buffer
	if err := WriteBacktestCSV(&output, csvResult()); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&output).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"expiry", "underlying", "strike", "option_type", "side", "entry_timestamp", "exit_timestamp", "entry_price", "exit_price", "quantity", "pnl"},
		{"2026-01-27", "NIFTY", "25000.05", "CALL", "SHORT", "2026-01-01T09:15:00+05:30", "2026-01-06T09:20:00+05:30", "127.05", "80.10", "75", "3521.25"},
		{"2026-01-27", "NIFTY", "25000.05", "CALL", "LONG", "2026-01-07T09:15:00+05:30", "", "0.29", "", "1", ""},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows=%v; want %v", rows, want)
	}
}

func TestCSVEmptyResultAndEscaping(t *testing.T) {
	var output bytes.Buffer
	if err := WriteBacktestCSV(&output, &engine.BacktestResult{}); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&output).ReadAll()
	if err != nil || len(rows) != 1 || len(rows[0]) != 11 {
		t.Fatalf("expected header only: %v %v", rows, err)
	}
	result := csvResult()
	result.Underlying = "NIFTY,\"INDEX\""
	if err := WriteBacktestCSV(&output, result); err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(&output).ReadAll()
	if err != nil || rows[1][1] != result.Underlying {
		t.Fatalf("CSV escaping failed: %v %v", rows, err)
	}
}

func TestCSVPropagatesWriteErrors(t *testing.T) {
	failure := errors.New("write failed")
	for _, symbol := range []string{"NIFTY", strings.Repeat("X", 10000)} {
		result := csvResult()
		result.Underlying = symbol
		if err := WriteBacktestCSV(errorWriter{failure}, result); !errors.Is(err, failure) {
			t.Fatalf("write error lost: %v", err)
		}
	}
}

func TestCSVPricePrecision(t *testing.T) {
	for _, tt := range []struct {
		price models.Price
		want  string
	}{
		{-12345, "-123.45"}, {models.Price(math.MaxInt64), "92233720368547758.07"},
		{models.Price(math.MinInt64), "-92233720368547758.08"},
	} {
		if got := formatPrice(tt.price); got != tt.want {
			t.Fatalf("formatPrice(%d)=%s; want %s", tt.price, got, tt.want)
		}
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }
