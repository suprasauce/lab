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

func marketTime(hour, minute int) time.Time {
	return time.Date(2026, 1, 5, hour, minute, 0, 0, marketLocation)
}

func executionFixture(t *testing.T, missing, missingUnderlying map[time.Time]bool) *Execution {
	t.Helper()
	var execution *Execution
	db := sql.OpenDB(testConnector{query: func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM lab.option_contracts") {
			return &testRows{columns: []string{"id", "underlying_id", "expiry_id", "expiry", "strike", "option_type"},
				values: []driver.Value{int64(101), int64(7), int64(9), execution.expiry, args[2].Value, "CALL"}}, nil
		}
		rows := &testRows{columns: []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}}
		start := args[2].Value.(time.Time)
		option := strings.Contains(query, "FROM lab.option_candles")
		if !option && !strings.Contains(query, "FROM lab.underlying_candles") {
			t.Fatalf("unexpected query: %s", query)
		}
		if !option && len(args) == 3 {
			t.Fatal("order execution must reuse the loaded underlying candle")
		}
		if option && args[3].Value.(time.Time).After(execution.Time) {
			t.Fatal("option lookup requests future data")
		}
		for _, minute := range []int{15, 20, 25, 30} {
			at := marketTime(9, minute)
			if at.Before(start) {
				continue
			}
			if len(args) == 4 {
				end := args[3].Value.(time.Time)
				if option && at.After(end) || !option && !at.Before(end) {
					continue
				}
			}
			if option && missing[at] || !option && missingUnderlying[at] {
				continue
			}
			price := int64(2250000 + (minute-15)*1000)
			if option {
				price = int64(10000 - (minute-15)*100)
			}
			rows.records = append(rows.records, []driver.Value{at, "5m", price, int64(9999999), int64(1), int64(7777777), int64(0)})
			if len(args) == 3 {
				break
			}
		}
		return rows, nil
	}})
	t.Cleanup(func() { db.Close() })
	repo := repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	execution = newExecution(context.Background(), repo, 7, time.Date(2026, 1, 27, 0, 0, 0, 0, marketLocation))
	return execution
}

func TestMarketOrderUsesPlacedAt(t *testing.T) {
	for _, tt := range []struct{ placedMinute, fillMinute, processedMinute int }{{15, 15, 20}, {20, 20, 25}} {
		t.Run(marketTime(9, tt.placedMinute).Format("15:04"), func(t *testing.T) {
			e := executionFixture(t, nil, nil)
			placedAt := marketTime(9, tt.placedMinute)
			if err := e.advance(placedAt); err != nil {
				t.Fatal(err)
			}
			if err := e.SellCall("CE", 75, 2); err != nil {
				t.Fatal(err)
			}
			if !e.orders["CE"].PlacedAt.Equal(placedAt) || len(e.Legs[0].Trades) != 0 {
				t.Fatal("entry did not queue with the simulation timestamp")
			}
			if err := e.advance(marketTime(9, tt.processedMinute)); err != nil {
				t.Fatal(err)
			}
			trade := e.Legs[0].Trades[0]
			wantPrice := models.Price(10000 - (tt.fillMinute-15)*100)
			if !trade.EntryTime.Equal(marketTime(9, tt.fillMinute)) || trade.EntryPrice != wantPrice {
				t.Fatalf("wrong fill: %+v", trade)
			}
			if e.Legs[0].Candle != nil && e.Legs[0].Candle.Timestamp.Add(candleInterval).After(e.Time) {
				t.Fatal("unfinished fill candle exposed to strategy")
			}
			if err := e.fillOrders(); err != nil {
				t.Fatal(err)
			}
			if len(e.Legs[0].Trades) != 1 || len(e.orders) != 0 {
				t.Fatal("order filled twice")
			}
		})
	}
}

func TestQueuedOrdersFillOnlyOnNextAdvance(t *testing.T) {
	e := executionFixture(t, nil, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if err := e.SellCall("CE", 75, 2); err != nil {
		t.Fatal(err)
	}
	if len(e.Legs[0].Trades) != 0 || len(e.orders) != 1 {
		t.Fatal("script request must only queue the entry")
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	leg := e.Legs[0]
	if leg.Status != models.LegOpen || !leg.Trades[0].EntryTime.Equal(marketTime(9, 15)) || leg.Trades[0].EntryPrice != 10000 {
		t.Fatalf("09:20 processing should record the eligible 09:15 Open: %+v", leg.Trades)
	}
	if leg.Contract.StrikePrice != 2260000 || leg.Candle == nil || !leg.Candle.Timestamp.Equal(marketTime(9, 15)) {
		t.Fatal("selection must use the 09:15 underlying Open and publish only its completed option candle")
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	if err := e.Close("CE"); err != nil {
		t.Fatal(err)
	}
	if leg.Trades[0].Status != models.TradeOpen || !leg.Trades[0].ExitTime.IsZero() {
		t.Fatal("close executed inside the callback")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	if leg.Trades[0].Status != models.TradeClosed || !leg.Trades[0].ExitTime.Equal(marketTime(9, 25)) || leg.Trades[0].ExitPrice != 9000 {
		t.Fatalf("09:30 processing should record the eligible 09:25 exit Open: %+v", leg.Trades[0])
	}
	if err := e.advance(marketTime(9, 35)); err != nil {
		t.Fatal(err)
	}
	if len(leg.Trades) != 1 || len(e.orders) != 0 {
		t.Fatal("order filled twice")
	}
}

func TestCompletedCandlesAndPositionBeforeNextEvaluation(t *testing.T) {
	e := executionFixture(t, nil, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if e.UnderlyingCandle != nil {
		t.Fatal("09:15 exposes an unfinished candle")
	}
	if err := e.SellCall("CE", 75, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	if e.Legs[0].Status != models.LegOpen || !e.UnderlyingCandle.Timestamp.Equal(marketTime(9, 15)) || !e.Legs[0].Candle.Timestamp.Equal(marketTime(9, 15)) {
		t.Fatal("09:20 evaluation must see the filled position and completed 09:15 candles")
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	if !e.UnderlyingCandle.Timestamp.Equal(marketTime(9, 20)) || !e.Legs[0].Candle.Timestamp.Equal(marketTime(9, 20)) {
		t.Fatal("09:25 must expose 09:20-09:25, not 09:25-09:30")
	}
	if err := e.Close("CE"); err != nil {
		t.Fatal(err)
	}
	if !e.orders["CE"].PlacedAt.Equal(marketTime(9, 25)) {
		t.Fatal("close has incorrect PlacedAt")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	trade := e.Legs[0].Trades[0]
	if !trade.ExitTime.Equal(marketTime(9, 25)) || trade.ExitPrice != 9000 || trade.Status != models.TradeClosed {
		t.Fatalf("incorrect close fill: %+v", trade)
	}
	if !e.Legs[0].Candle.Timestamp.Equal(marketTime(9, 25)) {
		t.Fatal("close exposed its unfinished fill candle")
	}
}

func TestMissingOptionBarWaitsWithoutLeakOrDuplicate(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 20): true}, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if err := e.SellCall("CE", 75, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	if e.Legs[0].Contract != nil || len(e.orders) != 1 {
		t.Fatal("entry should wait until a completed underlying candle is loaded")
	}
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	selected := e.Legs[0].Contract
	if selected == nil || selected.StrikePrice != 2260000 {
		t.Fatal("entry did not select from the loaded underlying candle")
	}
	if len(e.Legs[0].Trades) != 0 || e.Legs[0].Candle != nil || len(e.orders) != 1 {
		t.Fatal("future bar filled or exposed before 09:25")
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	if e.Legs[0].Status != models.LegOpen || !e.Legs[0].Trades[0].EntryTime.Equal(marketTime(9, 25)) || e.Legs[0].Candle != nil {
		t.Fatal("pending order should fill at 09:25 while its candle remains unavailable")
	}
	if e.Legs[0].Contract != selected || e.Legs[0].Contract.StrikePrice != 2260000 {
		t.Fatal("pending entry reselected its contract as the underlying price changed")
	}
	if err := e.SellCall("CE", 75, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	if len(e.Legs[0].Trades) != 1 || !e.Legs[0].Candle.Timestamp.Equal(marketTime(9, 25)) {
		t.Fatal("duplicate fill or wrong completed candle")
	}
}

func TestStrikeSelectionUsesLoadedUnderlyingOpen(t *testing.T) {
	e := executionFixture(t, nil, nil)
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	if e.UnderlyingCandle.Open != 2250000 {
		t.Fatal("expected the completed 09:15 candle")
	}
	if err := e.SellCall("CE", 75, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	// At 09:25, the loaded 09:20 Open is 22550 rupees; ATM+2 is 22650.
	if e.Legs[0].Contract.StrikePrice != 2265000 {
		t.Fatalf("strike did not use the loaded Open: %+v", e.Legs[0].Contract)
	}
	if !e.UnderlyingCandle.Timestamp.Equal(marketTime(9, 20)) {
		t.Fatal("strike selection published the unfinished current candle")
	}
}

func TestMissingLoadedUnderlyingKeepsOrderPending(t *testing.T) {
	e := executionFixture(t, nil, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if e.UnderlyingCandle != nil {
		t.Fatal("no completed underlying candle should exist at session start")
	}
	if err := e.SellCall("CE", 75, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	if e.Legs[0].Contract != nil || len(e.Legs[0].Trades) != 0 || len(e.orders) != 1 {
		t.Fatal("missing snapshot should leave the order unselected and pending")
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	if e.Legs[0].Contract.StrikePrice != 2260000 || !e.Legs[0].Trades[0].EntryTime.Equal(marketTime(9, 15)) {
		t.Fatal("order did not resume when the completed underlying candle became available")
	}
}

func TestDelayedSelectionDoesNotFillBeforeSelectionTime(t *testing.T) {
	e := executionFixture(t, nil, map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 20): true})
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if err := e.SellCall("CE", 75, 2); err != nil {
		t.Fatal(err)
	}
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	e.Time = marketTime(9, 20)
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	if e.Legs[0].Contract != nil || len(e.Legs[0].Trades) != 0 || e.UnderlyingCandle != nil {
		t.Fatal("future underlying candle was used before its timestamp")
	}
	if !e.orders["CE"].PlacedAt.Equal(marketTime(9, 15)) {
		t.Fatal("original placement time changed")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	trade := e.Legs[0].Trades[0]
	if !trade.EntryTime.Equal(marketTime(9, 25)) || trade.EntryPrice != 9000 {
		t.Fatalf("selected contract filled retroactively using an earlier candle: %+v", trade)
	}
}

func TestAdvanceClearsSnapshotsAndErrorsOnMissingCompletedCandles(t *testing.T) {
	for _, underlyingMissing := range []bool{true, false} {
		missingOption, missingUnderlying := map[time.Time]bool{}, map[time.Time]bool{}
		want := "missing completed option candle for leg CE contract 101"
		if underlyingMissing {
			missingUnderlying[marketTime(9, 20)] = true
			want = "missing completed underlying candle for underlying 7"
		} else {
			missingOption[marketTime(9, 20)] = true
		}
		e := executionFixture(t, missingOption, missingUnderlying)
		if err := e.advance(marketTime(9, 15)); err != nil {
			t.Fatal(err)
		}
		if err := e.SellCall("CE", 75, 2); err != nil {
			t.Fatal(err)
		}
		if err := e.fillOrders(); err != nil {
			t.Fatal(err)
		}
		if err := e.advance(marketTime(9, 20)); err != nil {
			t.Fatal(err)
		}
		if e.UnderlyingCandle == nil || e.Legs[0].Candle == nil {
			t.Fatal("expected previous snapshots")
		}
		err := e.advance(marketTime(9, 25))
		if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), marketTime(9, 20).Format(time.RFC3339)) {
			t.Fatalf("missing completed candle error lacks context: %v", err)
		}
		if e.Legs[0].Candle != nil || underlyingMissing && e.UnderlyingCandle != nil {
			t.Fatal("previous candle was retained")
		}
		if e.Legs[0].Status != models.LegOpen || len(e.Legs[0].Trades) != 1 {
			t.Fatal("candle reset changed position history")
		}
	}
}
