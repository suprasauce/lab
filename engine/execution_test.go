package engine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

func marketTime(hour, minute int) time.Time {
	return time.Date(2026, 1, 5, hour, minute, 0, 0, marketLocation)
}

func executionFixture(t *testing.T, missing map[time.Time]bool) *Execution {
	t.Helper()
	return executionFixtureWithLotSize(t, missing, 75)
}

func executionFixtureWithLotSize(t *testing.T, missing map[time.Time]bool, lotSize int64) *Execution {
	t.Helper()
	var execution *Execution
	db := sql.OpenDB(testConnector{query: func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM lab.underlyings") {
			rows := &testRows{columns: []string{"id", "symbol"}}
			if args[0].Value == "NIFTY" {
				rows.values = []driver.Value{int64(7), "NIFTY"}
			}
			return rows, nil
		}
		if strings.Contains(query, "FROM lab.option_contracts") {
			if !strings.Contains(query, "c.tradingsymbol = $1") || len(args) != 1 {
				t.Fatalf("unexpected contract lookup: %s", query)
			}
			rows := &testRows{columns: []string{"id", "underlying_id", "expiry_id", "expiry", "strike", "option_type", "underlying_symbol", "lot_size"}}
			kind, id := "CALL", int64(101)
			switch args[0].Value {
			case "NIFTY-20260127-22600-CE":
			case "NIFTY-20260127-22600-PE":
				kind, id = "PUT", 104
			default:
				return rows, nil
			}
			rows.values = []driver.Value{id, int64(7), int64(9), time.Date(2026, 1, 27, 0, 0, 0, 0, marketLocation), int64(2260000), kind, "NIFTY", lotSize}
			return rows, nil
		}
		rows := &testRows{columns: []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}}
		start := args[2].Value.(time.Time)
		underlying := strings.Contains(query, "FROM lab.underlying_candles")
		if !underlying && !strings.Contains(query, "FROM lab.option_candles") {
			t.Fatalf("unexpected query: %s", query)
		}
		if args[1].Value != "1m" {
			t.Fatal("expected one-minute candles")
		}
		end := args[3].Value.(time.Time)

		for _, minute := range []int{15, 16, 17, 18} {
			at := marketTime(9, minute)
			if at.Before(start) {
				continue
			}
			if at.After(end) || underlying && at.Equal(end) || missing[at] {
				continue
			}
			price := int64(10000 - (minute-15)*500)
			rows.records = append(rows.records, []driver.Value{at, "1m", price, int64(9999999), int64(1), int64(7777777), int64(0)})
		}
		return rows, nil
	}})
	t.Cleanup(func() { db.Close() })
	repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	execution = newExecution(context.Background(), repo, "1")
	return execution
}

func TestImmediateOrderAccounting(t *testing.T) {
	for _, symbol := range []string{"NIFTY", "NIFTY-20260127-22600-CE", "NIFTY-20260127-22600-PE"} {
		for _, quantity := range []int64{-2, 2} {
			t.Run(symbol+"/"+strconv.FormatInt(quantity, 10), func(t *testing.T) {
				e := executionFixtureWithLotSize(t, nil, 65)
				e.Time = marketTime(9, 15)
				id, err := e.PlaceTrade(symbol, quantity, 12345)
				if err != nil {
					t.Fatal(err)
				}
				p := e.Positions[0]
				if id != p.Trades[0].ID || p.Trades[0].Price != 12345 || !p.Trades[0].Timestamp.Equal(e.Time) || p.Quantity != quantity || p.AveragePrice != 12345 {
					t.Fatal("order did not execute immediately at supplied price")
				}
				e.Time = marketTime(9, 16)
				if _, err := e.PlaceTrade(symbol, -quantity, 13000); err != nil {
					t.Fatal(err)
				}
				multiplier := models.Price(65)
				if symbol == "NIFTY" {
					multiplier = 1
				}
				if p.Quantity != 0 || p.UnrealizedPnL != 0 || p.RealizedPnL != (13000-12345)*models.Price(quantity)*multiplier || len(p.Trades) != 2 {
					t.Fatal("incorrect exit accounting")
				}
				if _, err := e.PlaceTrade(symbol, quantity, 12000); err != nil {
					t.Fatal(err)
				}
				if len(e.Positions) != 1 || len(p.Trades) != 3 || p.AveragePrice != 12000 {
					t.Fatal("reopen did not reuse the position")
				}
				if p.Trades[0].ID == p.Trades[1].ID || p.Trades[0].SimulationID != e.ID {
					t.Fatal("incorrect trade identity")
				}
			})
		}
	}
}

func TestCurrentCandleAndMissingData(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 16): true})
	e.Time = marketTime(9, 15)
	candle, err := e.GetCandle("NIFTY")
	if err != nil || candle == nil || !candle.Timestamp.Equal(e.Time) || candle.Open != 10000 || candle.High != 9999999 || candle.Low != 1 || candle.Close != 7777777 {
		t.Fatalf("wrong current OHLC: %+v %v", candle, err)
	}
	if _, err := e.PlaceTrade("NIFTY", 1, 10000); err != nil {
		t.Fatal(err)
	}
	p := e.Positions[0]
	if err := e.advance(marketTime(9, 16)); err != nil {
		t.Fatal(err)
	}
	if p.Candle != nil || len(p.TotalPnL) != 2 || p.UnrealizedPnL != 7777777-10000 {
		t.Fatal("finished candle was not marked independently of missing current candle")
	}
	if candle, err = e.GetCandle("NIFTY"); err != nil || candle != nil {
		t.Fatal("missing candle should return nil")
	}
	if err := e.advance(marketTime(9, 17)); err != nil {
		t.Fatal(err)
	}
	if p.Candle == nil || len(p.TotalPnL) != 2 || p.UnrealizedPnL != 7777777-10000 {
		t.Fatal("missing finished candle should preserve P&L and skip its history point")
	}
	// A script-supplied execution price does not require a historical fill candle.
	if _, err := e.PlaceTrade("NIFTY", -1, 11000); err != nil {
		t.Fatal(err)
	}
	if p.Quantity != 0 || p.RealizedPnL != 1000 {
		t.Fatal("explicit-price exit was deferred")
	}
	if err := e.advance(marketTime(9, 17)); err != nil {
		t.Fatal(err)
	}
	if len(p.Trades) != 2 {
		t.Fatal("advance created duplicate fills")
	}
}

func TestImmediateOrderRejectsInvalidChangesWithoutRecording(t *testing.T) {
	e := executionFixture(t, nil)
	e.Time = marketTime(9, 15)
	for _, tt := range []struct {
		symbol   string
		quantity int64
		price    models.Price
	}{
		{"", 1, 100}, {"NIFTY", 0, 100}, {"NIFTY", 1, -1}, {"UNKNOWN", 1, 100},
	} {
		if _, err := e.PlaceTrade(tt.symbol, tt.quantity, tt.price); err == nil {
			t.Fatal("invalid order accepted")
		}
	}
	if len(e.Positions) != 0 {
		t.Fatal("failed order modified execution")
	}
	if _, err := e.PlaceTrade("NIFTY", 2, 100); err != nil {
		t.Fatal(err)
	}
	for _, quantity := range []int64{1, -1, -3} {
		if _, err := e.PlaceTrade("NIFTY", quantity, 100); err == nil {
			t.Fatal("unsupported position change accepted")
		}
	}
	if len(e.Positions[0].Trades) != 1 || e.Positions[0].Quantity != 2 {
		t.Fatal("rejected changes modified position")
	}
}

func TestUnrealizedPnLWaitsUntilCandleFinishes(t *testing.T) {
	for _, quantity := range []int64{-1, 1} {
		e := executionFixtureWithLotSize(t, nil, 65)
		e.Time = marketTime(9, 15)
		symbol := "NIFTY-20260127-22600-CE"
		if _, err := e.PlaceTrade(symbol, quantity, 10000); err != nil {
			t.Fatal(err)
		}
		p := e.Positions[0]
		if p.UnrealizedPnL != 0 || p.TotalPnL[0].PnL != 0 {
			t.Fatal("entry exposed unfinished Close in P&L")
		}
		// Deliberately make the current and previous closes different.
		p.Candle.Close = 11000
		if err := e.advance(marketTime(9, 16)); err != nil {
			t.Fatal(err)
		}
		want := models.Price(1000 * quantity * 65)
		if p.UnrealizedPnL != want || p.Candle == nil || !p.Candle.Timestamp.Equal(marketTime(9, 16)) || p.Candle.Close != 7777777 {
			t.Fatal("P&L should use previous Close while script sees current candle")
		}
		if len(p.TotalPnL) != 2 || !p.TotalPnL[1].Timestamp.Equal(marketTime(9, 16)) || p.TotalPnL[1].PnL != want {
			t.Fatal("incorrect completed-candle P&L history")
		}
		if _, err := e.PlaceTrade(symbol, -quantity, 12000); err != nil {
			t.Fatal(err)
		}
		if p.UnrealizedPnL != 0 || p.RealizedPnL != models.Price(2000*quantity*65) {
			t.Fatal("immediate close accounting changed")
		}
		if _, err := e.PlaceTrade(symbol, quantity, 12000); err != nil {
			t.Fatal(err)
		}
		if p.UnrealizedPnL != 0 {
			t.Fatal("reopened position used unfinished Close")
		}
	}
}

func TestFinalSessionCandleIsMarkedAt1530(t *testing.T) {
	e := executionFixture(t, nil)
	e.Time = marketTime(15, 29)
	if _, err := e.PlaceTrade("NIFTY", 1, 10000); err != nil {
		t.Fatal(err)
	}
	p := e.Positions[0]
	p.Candle = &models.Candle{Timestamp: e.Time, Close: 11000}
	if err := e.advance(marketTime(15, 30)); err != nil {
		t.Fatal(err)
	}
	if p.UnrealizedPnL != 1000 || p.Candle != nil {
		t.Fatal("final completed candle was not marked")
	}
}

func TestGetLotSizeUsesResolvedExpiry(t *testing.T) {
	for _, size := range []int64{25, 65, 75} {
		e := executionFixtureWithLotSize(t, nil, size)
		for _, symbol := range []string{"NIFTY-20260127-22600-CE", "NIFTY-20260127-22600-PE", "NIFTY"} {
			want := size
			if symbol == "NIFTY" {
				want = 1
			}
			got, err := e.GetLotSize(symbol)
			if err != nil || got != want {
				t.Fatalf("%s lot size=%d; want %d; error=%v", symbol, got, want, err)
			}
		}
		if _, err := e.GetLotSize("UNKNOWN"); err == nil {
			t.Fatal("unknown symbol must return an error")
		}
	}
}
