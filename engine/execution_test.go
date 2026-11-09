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
			if len(args) == 1 {
				rows := &testRows{columns: []string{"id", "underlying_id", "expiry_id", "expiry", "strike", "option_type"}}
				id := args[0].Value.(int64)
				if id >= 101 && id <= 103 {
					underlyingID, expiry := int64(7), execution.expiry
					if id == 102 {
						underlyingID = 8
					}
					if id == 103 {
						expiry = expiry.AddDate(0, 1, 0)
					}
					rows.values = []driver.Value{id, underlyingID, int64(9), expiry, int64(2260000), "CALL"}
				}
				return rows, nil
			}
			return &testRows{columns: []string{"id", "underlying_id", "expiry_id", "expiry", "strike", "option_type"},
				values: []driver.Value{int64(101), int64(7), int64(9), execution.expiry, args[2].Value, args[3].Value}}, nil
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
		if option && args[3].Value.(time.Time).After(execution.Time.Add(-candleInterval)) {
			t.Fatal("option lookup requests an unfinished candle")
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
	repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	execution = newExecution(context.Background(), repo, 7, time.Date(2026, 1, 27, 0, 0, 0, 0, marketLocation))
	return execution
}

func atmRequest() models.ContractRequest {
	return models.ContractRequest{OptionType: models.Call, Offset: 2}
}

func TestMarketOrderUsesPlacedAt(t *testing.T) {
	for _, minute := range []int{15, 20} {
		t.Run(marketTime(9, minute).Format("15:04"), func(t *testing.T) {
			e := executionFixture(t, nil, nil)
			if err := e.advance(marketTime(9, minute)); err != nil {
				t.Fatal(err)
			}
			id, err := e.PlaceOrder(atmRequest(), "SELL", 75)
			if err != nil {
				t.Fatal(err)
			}
			order, _ := e.Order(id)
			if len(e.Positions) != 0 || order.Contract != nil || order.Status != models.OrderPending || !order.PlacedAt.Equal(e.Time) {
				t.Fatal("placement performed execution")
			}
			if err := e.advance(marketTime(9, minute+5)); err != nil {
				t.Fatal(err)
			}
			position := e.Positions[0]
			if !position.EntryTime.Equal(marketTime(9, minute)) || position.EntryPrice != models.Price(10000-(minute-15)*100) || order.Status != models.OrderFilled || !order.FilledAt.Equal(position.EntryTime) {
				t.Fatalf("incorrect fill: %+v", order)
			}
			if err := e.fillOrders(); err != nil {
				t.Fatal(err)
			}
			if len(e.Positions) != 1 {
				t.Fatal("duplicate fill")
			}
			if position.Candle != nil && position.Candle.Timestamp.Add(candleInterval).After(e.Time) {
				t.Fatal("future OHLC exposed")
			}
		})
	}
}

func TestPositionFullExitUsesLoadedCandle(t *testing.T) {
	e := executionFixture(t, nil, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	id, err := e.PlaceOrder(atmRequest(), "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	if !e.UnderlyingCandle.Timestamp.Equal(marketTime(9, 20)) || !e.Positions[0].Candle.Timestamp.Equal(marketTime(9, 20)) {
		t.Fatal("wrong completed candle")
	}
	entry, _ := e.Order(id)
	exitID, err := e.PlaceOrder(models.ContractRequest{ContractID: entry.Contract.ID}, "BUY", 75)
	if err != nil {
		t.Fatal(err)
	}
	if e.Positions[0].Status != models.TradeOpen {
		t.Fatal("exit executed during placement")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	position := e.Positions[0]
	exit, _ := e.Order(exitID)
	if position.Status != models.TradeClosed || !position.ExitTime.Equal(marketTime(9, 25)) || position.ExitPrice != 9000 || position.PnL != 75000 || exit.Status != models.OrderFilled {
		t.Fatalf("wrong exit: %+v", position)
	}
	if err := e.advance(marketTime(9, 35)); err != nil {
		t.Fatal(err)
	}
	if len(e.Positions) != 1 {
		t.Fatal("duplicate position")
	}
}

func TestEveryPlacementCreatesAnIndependentOrder(t *testing.T) {
	e := executionFixture(t, nil, nil)
	e.Time = marketTime(9, 15)
	first, err := e.PlaceOrder(atmRequest(), "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.PlaceOrder(atmRequest(), "SELL", 75)
	if err != nil || first == second || len(e.Orders) != 2 || len(e.Positions) != 0 {
		t.Fatal("requests were deduplicated or executed")
	}
	for _, order := range e.Orders {
		if order.Status != models.OrderPending || order.Contract != nil {
			t.Fatal("placement selected a contract")
		}
	}
}

func TestMissingOptionBarRetainsSelection(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 20): true}, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	id, err := e.PlaceOrder(atmRequest(), "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	selected := order.Contract
	if selected == nil || selected.StrikePrice != 2260000 || order.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("did not retain pending selection")
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	if order.Contract != selected || order.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("reselected or filled using the unfinished 09:25 candle")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	if order.Contract != selected || order.Status != models.OrderFilled || !order.FilledAt.Equal(marketTime(9, 25)) || !e.Positions[0].Candle.Timestamp.Equal(marketTime(9, 25)) {
		t.Fatal("order did not fill using the completed 09:25 candle at 09:30")
	}
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	if len(e.Positions) != 1 {
		t.Fatal("duplicate fill")
	}
}

func TestStrikeSelectionUsesNextAdvanceLoadedOpen(t *testing.T) {
	e := executionFixture(t, nil, nil)
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	if e.UnderlyingCandle.Open != 2250000 {
		t.Fatal("wrong script snapshot")
	}
	id, err := e.PlaceOrder(atmRequest(), "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	if order.Contract != nil {
		t.Fatal("selected from script's snapshot")
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	if order.Contract.StrikePrice != 2265000 {
		t.Fatalf("wrong execution-time strike: %+v", order.Contract)
	}
}

func TestMissingLoadedUnderlyingKeepsOrderPending(t *testing.T) {
	e := executionFixture(t, nil, map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 20): true})
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	id, err := e.PlaceOrder(atmRequest(), "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	if order.Contract != nil || len(e.Positions) != 0 {
		t.Fatal("missing underlying did not leave order pending")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	if !order.FilledAt.Equal(marketTime(9, 25)) || !order.PlacedAt.Equal(marketTime(9, 15)) {
		t.Fatal("retroactive fill or changed placement time")
	}
}

func TestAdvanceClearsSnapshotsAndContinuesOnMissingCompletedCandles(t *testing.T) {
	for _, underlyingMissing := range []bool{true, false} {
		missingOption, missingUnderlying := map[time.Time]bool{}, map[time.Time]bool{}
		if underlyingMissing {
			missingUnderlying[marketTime(9, 20)] = true
		} else {
			missingOption[marketTime(9, 20)] = true
		}
		e := executionFixture(t, missingOption, missingUnderlying)
		if err := e.advance(marketTime(9, 15)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PlaceOrder(atmRequest(), "SELL", 75); err != nil {
			t.Fatal(err)
		}
		if err := e.advance(marketTime(9, 20)); err != nil {
			t.Fatal(err)
		}
		if err := e.advance(marketTime(9, 25)); err != nil {
			t.Fatal(err)
		}
		if underlyingMissing && e.UnderlyingCandle != nil || !underlyingMissing && e.Positions[0].Candle != nil || e.Positions[0].Status != models.TradeOpen {
			t.Fatal("stale snapshot or changed position")
		}
		if underlyingMissing && (e.Positions[0].Candle == nil || !e.Positions[0].Candle.Timestamp.Equal(marketTime(9, 20))) {
			t.Fatal("missing underlying prevented option candle loading")
		}
		if err := e.advance(marketTime(9, 30)); err != nil {
			t.Fatal(err)
		}
		if e.UnderlyingCandle == nil || e.Positions[0].Candle == nil || !e.UnderlyingCandle.Timestamp.Equal(marketTime(9, 25)) || !e.Positions[0].Candle.Timestamp.Equal(marketTime(9, 25)) {
			t.Fatal("subsequent completed candles were not loaded")
		}
	}
}

func TestMissingOptionCandleDefersExit(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 20): true}, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.PlaceOrder(atmRequest(), "SELL", 75); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	position := e.Positions[0]
	id, err := e.PlaceOrder(models.ContractRequest{ContractID: position.ContractID}, "BUY", 75)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	if position.Candle != nil || position.Status != models.TradeOpen || order.Status != models.OrderPending || !position.ExitTime.IsZero() {
		t.Fatal("missing candle used a stale snapshot or filled the exit")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	if position.Status != models.TradeClosed || !position.ExitTime.Equal(marketTime(9, 25)) || position.ExitPrice != 9000 || order.Status != models.OrderFilled {
		t.Fatal("exit did not fill with the next available completed candle")
	}
	if err := e.fillOrders(); err != nil || len(e.Positions) != 1 {
		t.Fatalf("duplicate fill or error: %v", err)
	}
}

func TestAdvancePropagatesCandleQueryFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	db := sql.OpenDB(testConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) { return nil, failure }})
	defer db.Close()
	repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e := newExecution(context.Background(), repo, 7, marketTime(0, 0))
	if err := e.advance(marketTime(9, 20)); !errors.Is(err, failure) {
		t.Fatalf("database failure was not propagated: %v", err)
	}
}

func TestOptionTypesAndPositionSides(t *testing.T) {
	for _, optionType := range []models.OptionType{models.Call, models.Put} {
		for _, side := range []models.PositionSide{models.Long, models.Short} {
			t.Run(string(optionType)+"/"+string(side), func(t *testing.T) {
				e := executionFixture(t, nil, nil)
				if err := e.advance(marketTime(9, 15)); err != nil {
					t.Fatal(err)
				}
				entrySide, exitSide := "SELL", "BUY"
				if side == models.Long {
					entrySide, exitSide = "BUY", "SELL"
				}
				if _, err := e.PlaceOrder(models.ContractRequest{OptionType: optionType, Offset: 2}, entrySide, 75); err != nil {
					t.Fatal(err)
				}
				if err := e.advance(marketTime(9, 20)); err != nil {
					t.Fatal(err)
				}
				position := e.Positions[0]
				if position.Contract.OptionType != optionType || position.Side != side {
					t.Fatal("incorrect option or side")
				}
				if _, err := e.PlaceOrder(models.ContractRequest{ContractID: position.ContractID}, exitSide, 75); err != nil {
					t.Fatal(err)
				}
				if err := e.advance(marketTime(9, 25)); err != nil {
					t.Fatal(err)
				}
				want := models.Price(37500)
				if side == models.Long {
					want = -want
				}
				if position.Status != models.TradeClosed || position.PnL != want {
					t.Fatalf("incorrect P&L: %+v", position)
				}
			})
		}
	}
}

func TestExactContractIDAndInvalidID(t *testing.T) {
	for _, id := range []int64{101, 404} {
		e := executionFixture(t, nil, nil)
		if err := e.advance(marketTime(9, 15)); err != nil {
			t.Fatal(err)
		}
		orderID, err := e.PlaceOrder(models.ContractRequest{ContractID: id}, "BUY", 75)
		if err != nil {
			t.Fatal(err)
		}
		order, _ := e.Order(orderID)
		if order.Contract != nil || order.Status != models.OrderPending || len(e.Positions) != 0 {
			t.Fatal("exact contract was resolved during placement")
		}
		err = e.advance(marketTime(9, 20))
		if id == 404 {
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected missing ID, got %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if order.Contract.ID != 101 || order.Status != models.OrderFilled || e.Positions[0].Side != models.Long {
			t.Fatal("incorrect exact-ID fill")
		}
	}
}

func TestUnsupportedPositionChangesFailAtExecution(t *testing.T) {
	for _, request := range []struct {
		side     string
		quantity int64
	}{{"BUY", 1}, {"BUY", 100}, {"SELL", 75}} {
		e := executionFixture(t, nil, nil)
		if err := e.advance(marketTime(9, 15)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PlaceOrder(atmRequest(), "SELL", 75); err != nil {
			t.Fatal(err)
		}
		if err := e.advance(marketTime(9, 20)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PlaceOrder(models.ContractRequest{ContractID: 101}, request.side, request.quantity); err != nil {
			t.Fatal("placement should only queue", err)
		}
		if err := e.advance(marketTime(9, 25)); err == nil {
			t.Fatal("unsupported position change executed")
		}
		if e.Positions[0].Status != models.TradeOpen || e.Positions[0].Quantity != 75 {
			t.Fatal("position mutated")
		}
	}
}

func TestPendingOrdersForSameContractAreAttemptedIndependently(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 20): true}, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	entryID, err := e.PlaceOrder(atmRequest(), "SELL", 75)
	if err != nil {
		t.Fatal(err)
	}
	exitID, err := e.PlaceOrder(models.ContractRequest{ContractID: 101}, "BUY", 75)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 20)); err != nil {
		t.Fatal(err)
	}
	entry, _ := e.Order(entryID)
	exit, _ := e.Order(exitID)
	if entry.Status != models.OrderPending || exit.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("orders should remain pending while option candles are unavailable")
	}
	if entry.Contract == nil || exit.Contract == nil {
		t.Fatal("both orders must resolve their contracts independently even when the earlier order is pending")
	}
	if err := e.advance(marketTime(9, 25)); err != nil {
		t.Fatal(err)
	}
	if entry.Status != models.OrderPending || exit.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("orders must wait until the 09:25 candle completes")
	}
	if err := e.advance(marketTime(9, 30)); err != nil {
		t.Fatal(err)
	}
	if exit.Status != models.OrderFilled || e.Positions[0].Status != models.TradeClosed || !entry.FilledAt.Equal(marketTime(9, 25)) || !exit.FilledAt.Equal(marketTime(9, 25)) {
		t.Fatal("incorrect full exit after delayed entry")
	}
}

func TestExactContractMustMatchUnderlyingAndExpiry(t *testing.T) {
	for _, id := range []int64{102, 103} {
		e := executionFixture(t, nil, nil)
		if err := e.advance(marketTime(9, 15)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PlaceOrder(models.ContractRequest{ContractID: id}, "BUY", 75); err != nil {
			t.Fatal(err)
		}
		err := e.advance(marketTime(9, 20))
		if err == nil || !strings.Contains(err.Error(), "current underlying and expiry") || len(e.Positions) != 0 {
			t.Fatalf("foreign contract was accepted: %v", err)
		}
	}
}
