package engine

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
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

func executionFixture(t *testing.T, missing map[time.Time]bool) *Execution {
	t.Helper()
	return executionFixtureWithLotSize(t, missing, 75)
}

func executionFixtureWithLotSize(t *testing.T, missing map[time.Time]bool, lotSize int64) *Execution {
	t.Helper()
	var execution *Execution
	db := sql.OpenDB(testConnector{query: func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
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
		if !strings.Contains(query, "FROM lab.option_candles") {
			t.Fatalf("unexpected query: %s", query)
		}
		if !args[3].Value.(time.Time).Before(execution.Time) {
			t.Fatal("option lookup requests an unfinished candle")
		}
		for _, minute := range []int{15, 16, 17, 18} {
			at := marketTime(9, minute)
			if at.Before(start) {
				continue
			}
			if at.After(args[3].Value.(time.Time)) || missing[at] {
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

func TestMarketOrderUsesPlacedAt(t *testing.T) {
	for _, minute := range []int{15, 16, 17} {
		t.Run(marketTime(9, minute).Format("15:04"), func(t *testing.T) {
			e := executionFixture(t, nil)
			if err := e.advance(marketTime(9, minute)); err != nil {
				t.Fatal(err)
			}
			id, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1)
			if err != nil {
				t.Fatal(err)
			}
			order, _ := e.Order(id)
			if len(e.Positions) != 0 || order.Contract != nil || order.Status != models.OrderPending || !order.PlacedAt.Equal(e.Time) {
				t.Fatal("placement performed execution")
			}
			if err := e.advance(marketTime(9, minute+1)); err != nil {
				t.Fatal(err)
			}
			position := e.Positions[0]
			if !e.Positions[0].Trades[0].Timestamp.Equal(marketTime(9, minute)) || position.AveragePrice != models.Price(10000-(minute-15)*500) || order.Status != models.OrderFilled || !order.FilledAt.Equal(e.Positions[0].Trades[0].Timestamp) {
				t.Fatalf("incorrect fill: %+v", order)
			}
			if err := e.fillOrders(); err != nil {
				t.Fatal(err)
			}
			if len(e.Positions) != 1 || len(e.Positions[0].Trades) != 1 {
				t.Fatal("duplicate fill")
			}
			if position.Candle != nil && position.Candle.Timestamp.Add(candleInterval).After(e.Time) {
				t.Fatal("future OHLC exposed")
			}
		})
	}
}

func TestPositionFullExitUsesLoadedCandle(t *testing.T) {
	e := executionFixture(t, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	_, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 16)); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 17)); err != nil {
		t.Fatal(err)
	}
	if !e.Positions[0].Candle.Timestamp.Equal(marketTime(9, 16)) {
		t.Fatal("wrong completed candle")
	}
	exitID, err := e.PlaceOrder("NIFTY-20260127-22600-CE", 1)
	if err != nil {
		t.Fatal(err)
	}
	if e.Positions[0].Quantity == 0 {
		t.Fatal("exit executed during placement")
	}
	if err := e.advance(marketTime(9, 18)); err != nil {
		t.Fatal(err)
	}
	position := e.Positions[0]
	exit, _ := e.Order(exitID)
	if position.Quantity != 0 || !e.Positions[0].Trades[1].Timestamp.Equal(marketTime(9, 17)) || e.Positions[0].Trades[1].Price != 9000 || (e.Positions[0].Trades[1].Price-e.Positions[0].Trades[0].Price)*models.Price(e.Positions[0].Trades[0].Quantity)*75 != 75000 || exit.Status != models.OrderFilled {
		t.Fatalf("wrong exit: %+v", position)
	}
	if err := e.advance(marketTime(9, 19)); err != nil {
		t.Fatal(err)
	}
	if len(e.Positions) != 1 {
		t.Fatal("duplicate position")
	}
}

func TestEveryPlacementCreatesAnIndependentOrder(t *testing.T) {
	e := executionFixture(t, nil)
	e.Time = marketTime(9, 15)
	first, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1)
	if err != nil || first == second || len(e.Orders) != 2 || len(e.Positions) != 0 {
		t.Fatal("requests were deduplicated or executed")
	}
	for _, order := range e.Orders {
		if order.Status != models.OrderPending || order.Contract != nil {
			t.Fatal("placement resolved a contract")
		}
	}
}

func TestMissingOptionBarRetainsResolvedContract(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 16): true})
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	id, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 16)); err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	selected := order.Contract
	if selected == nil || selected.StrikePrice != 2260000 || order.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("did not retain pending contract")
	}
	if err := e.advance(marketTime(9, 17)); err != nil {
		t.Fatal(err)
	}
	if order.Contract != selected || order.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("changed contract or filled using the unfinished 09:17 candle")
	}
	if err := e.advance(marketTime(9, 18)); err != nil {
		t.Fatal(err)
	}
	if order.Contract != selected || order.Status != models.OrderFilled || !order.FilledAt.Equal(marketTime(9, 17)) || !e.Positions[0].Candle.Timestamp.Equal(marketTime(9, 17)) {
		t.Fatal("order did not fill using the completed 09:17 candle at 09:18")
	}
	if err := e.fillOrders(); err != nil {
		t.Fatal(err)
	}
	if len(e.Positions) != 1 {
		t.Fatal("duplicate fill")
	}
}

func TestAdvanceClearsSnapshotsAndContinuesOnMissingCompletedCandles(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 16): true})
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 16)); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 17)); err != nil {
		t.Fatal(err)
	}
	if e.Positions[0].Candle != nil || e.Positions[0].Quantity == 0 {
		t.Fatal("stale snapshot or changed position")
	}
	if err := e.advance(marketTime(9, 18)); err != nil {
		t.Fatal(err)
	}
	if e.Positions[0].Candle == nil || !e.Positions[0].Candle.Timestamp.Equal(marketTime(9, 17)) {
		t.Fatal("subsequent completed candle was not loaded")
	}
}

func TestMissingOptionCandleDefersExit(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 16): true})
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1); err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 16)); err != nil {
		t.Fatal(err)
	}
	position := e.Positions[0]
	id, err := e.PlaceOrder("NIFTY-20260127-22600-CE", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 17)); err != nil {
		t.Fatal(err)
	}
	order, _ := e.Order(id)
	if position.Candle != nil || position.Quantity == 0 || order.Status != models.OrderPending || len(e.Positions[0].Trades) != 1 {
		t.Fatal("missing candle used a stale snapshot or filled the exit")
	}
	if err := e.advance(marketTime(9, 18)); err != nil {
		t.Fatal(err)
	}
	if position.Quantity != 0 || !e.Positions[0].Trades[1].Timestamp.Equal(marketTime(9, 17)) || e.Positions[0].Trades[1].Price != 9000 || order.Status != models.OrderFilled {
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
	e := newExecution(context.Background(), repo, "1")
	e.Positions = []*models.Position{{TradingSymbol: "NIFTY-20260127-22600-CE", Quantity: -1}}
	if err := e.advance(marketTime(9, 16)); !errors.Is(err, failure) {
		t.Fatalf("database failure was not propagated: %v", err)
	}
}

func TestOptionTypesAndPositionSides(t *testing.T) {
	for _, optionType := range []models.OptionType{models.Call, models.Put} {
		for _, side := range []models.PositionSide{models.Long, models.Short} {
			t.Run(string(optionType)+"/"+string(side), func(t *testing.T) {
				e := executionFixture(t, nil)
				tradingSymbol := map[models.OptionType]string{models.Call: "NIFTY-20260127-22600-CE", models.Put: "NIFTY-20260127-22600-PE"}[optionType]
				if err := e.advance(marketTime(9, 15)); err != nil {
					t.Fatal(err)
				}
				entryQuantity, exitQuantity := int64(-1), int64(1)
				if side == models.Long {
					entryQuantity, exitQuantity = 1, -1
				}
				if _, err := e.PlaceOrder(tradingSymbol, entryQuantity); err != nil {
					t.Fatal(err)
				}
				if err := e.advance(marketTime(9, 16)); err != nil {
					t.Fatal(err)
				}
				position := e.Positions[0]
				if got := e.GetPositions(tradingSymbol); len(got) != 1 || got[0] != position {
					t.Fatal("filled position not indexed by trading symbol")
				}
				if e.Orders[0].Contract.OptionType != optionType || position.Quantity != entryQuantity {
					t.Fatal("incorrect option or side")
				}
				if _, err := e.PlaceOrder(tradingSymbol, exitQuantity); err != nil {
					t.Fatal(err)
				}
				if err := e.advance(marketTime(9, 17)); err != nil {
					t.Fatal(err)
				}
				want := models.Price(37500)
				if side == models.Long {
					want = -want
				}
				if position.Quantity != 0 || position.RealizedPnL != want {
					t.Fatalf("incorrect P&L: %+v", position)
				}
			})
		}
	}
}

func TestUnsupportedPositionChangesFailAtExecution(t *testing.T) {
	for _, quantity := range []int64{2, -1} {
		e := executionFixture(t, nil)
		if err := e.advance(marketTime(9, 15)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1); err != nil {
			t.Fatal(err)
		}
		if err := e.advance(marketTime(9, 16)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PlaceOrder("NIFTY-20260127-22600-CE", quantity); err != nil {
			t.Fatal("placement should only queue", err)
		}
		if err := e.advance(marketTime(9, 17)); err == nil {
			t.Fatal("unsupported position change executed")
		}
		if e.Positions[0].Quantity == 0 || e.Positions[0].Quantity != -1 {
			t.Fatal("position mutated")
		}
	}
}

func TestPendingOrdersForSameContractAreAttemptedIndependently(t *testing.T) {
	e := executionFixture(t, map[time.Time]bool{marketTime(9, 15): true, marketTime(9, 16): true})
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	entryID, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1)
	if err != nil {
		t.Fatal(err)
	}
	exitID, err := e.PlaceOrder("NIFTY-20260127-22600-CE", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.advance(marketTime(9, 16)); err != nil {
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
	if err := e.advance(marketTime(9, 17)); err != nil {
		t.Fatal(err)
	}
	if entry.Status != models.OrderPending || exit.Status != models.OrderPending || len(e.Positions) != 0 {
		t.Fatal("orders must wait until the 09:17 candle completes")
	}
	if err := e.advance(marketTime(9, 18)); err != nil {
		t.Fatal(err)
	}
	if exit.Status != models.OrderFilled || e.Positions[0].Quantity != 0 || !entry.FilledAt.Equal(marketTime(9, 17)) || !exit.FilledAt.Equal(marketTime(9, 17)) {
		t.Fatal("incorrect full exit after delayed entry")
	}
}

func TestFillLedgerAndReopenedPosition(t *testing.T) {
	e := executionFixture(t, nil)
	symbol := "NIFTY-20260127-22600-CE"
	quantities := []int64{-1, 1, 1, -1}
	for i, quantity := range quantities {
		at := marketTime(9, 15+i)
		if i == 0 {
			if err := e.advance(at); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := e.PlaceOrder(symbol, quantity); err != nil {
			t.Fatal(err)
		}
		if i > 0 && len(e.Positions[0].Trades) != i {
			t.Fatal("queued order created a trade")
		}
		if err := e.advance(at.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if len(e.Positions[0].Trades) != i+1 {
			t.Fatal("one fill must create exactly one trade")
		}
		if len(e.Positions) != 1 {
			t.Fatal("reopening created a second position")
		}
		if (i == 1 || i == 2) && e.Positions[0].RealizedPnL != 37500 {
			t.Fatal("realized P&L was not retained across reopen")
		}
		current := e.Positions[0]
		last := current.TotalPnL[len(current.TotalPnL)-1]
		if last.PnL != current.RealizedPnL+current.UnrealizedPnL {
			t.Fatal("total P&L must include retained realized P&L after reopen")
		}
		fill := e.Positions[0].Trades[i]
		if fill.TradingSymbol != symbol || fill.SimulationID != "1" || fill.ID != fmt.Sprintf("1/%d", i+1) || fill.Quantity != quantity || !fill.Timestamp.Equal(at) || fill.Price != models.Price(10000-i*500) {
			t.Fatalf("incorrect fill: %+v", fill)
		}
	}
	positions := e.GetPositions(symbol)
	if len(positions) != 1 || positions[0].Quantity != 0 || positions[0].SimulationID != "1" {
		t.Fatalf("incorrect position history: %+v", positions)
	}
	if len(positions[0].TotalPnL) != 4 {
		t.Fatal("total P&L history was lost across closes or reopen")
	}
	if positions[0].AveragePrice != 9000 || positions[0].RealizedPnL != 0 {
		t.Fatal("reopened position did not preserve cumulative realized P&L")
	}
	if positions[0].Candle != nil {
		t.Fatal("closed position retains market candle")
	}
	if err := e.fillOrders(); err != nil || len(e.Positions[0].Trades) != 4 {
		t.Fatalf("duplicate trade or error: %v", err)
	}
}

func TestTradeIdentityAcrossSimulations(t *testing.T) {
	seen := make(map[string]bool)
	for _, simulationID := range []string{"1", "2"} {
		e := executionFixture(t, nil)
		e.ID = simulationID
		if err := e.advance(marketTime(9, 15)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PlaceOrder("NIFTY-20260127-22600-CE", -1); err != nil {
			t.Fatal(err)
		}
		if err := e.advance(marketTime(9, 16)); err != nil {
			t.Fatal(err)
		}
		position := e.Positions[0]
		trade := position.Trades[0]
		if position.SimulationID != simulationID || trade.SimulationID != simulationID || trade.TradingSymbol != position.TradingSymbol || seen[trade.ID] {
			t.Fatalf("incorrect simulation/position association: %+v %+v", position, trade)
		}
		seen[trade.ID] = true
	}
}

func TestExpiryLotSizeMultiplier(t *testing.T) {
	for _, lotSize := range []int64{25, 50, 65, 75} {
		for _, lots := range []int64{-2, 2} {
			t.Run(fmt.Sprintf("lot_size=%d/lots=%d", lotSize, lots), func(t *testing.T) {
				e := executionFixtureWithLotSize(t, nil, lotSize)
				symbol := "NIFTY-20260127-22600-CE"
				if err := e.advance(marketTime(9, 15)); err != nil {
					t.Fatal(err)
				}
				if _, err := e.PlaceOrder(symbol, lots); err != nil {
					t.Fatal(err)
				}
				if err := e.advance(marketTime(9, 16)); err != nil {
					t.Fatal(err)
				}
				position := e.Positions[0]
				if position.Quantity != lots || position.Trades[0].Quantity != lots || e.Orders[0].Quantity != lots || e.Orders[0].Contract.LotSize != lotSize {
					t.Fatal("quantities must remain in lots and contract must retain expiry lot size")
				}
				if _, err := e.PlaceOrder(symbol, -lots); err != nil {
					t.Fatal(err)
				}
				if err := e.advance(marketTime(9, 17)); err != nil {
					t.Fatal(err)
				}
				want := models.Price(-500 * lots * lotSize)
				if position.Quantity != 0 || position.RealizedPnL != want || position.Trades[1].Quantity != -lots {
					t.Fatalf("P&L=%d want=%d", position.RealizedPnL, want)
				}
			})
		}
	}
}

func TestPositionTotalPnLHistorySkipsMissingCandles(t *testing.T) {
	for _, lots := range []int64{-2, 2} {
		t.Run(fmt.Sprint(lots), func(t *testing.T) {
			e := executionFixtureWithLotSize(t, map[time.Time]bool{marketTime(9, 16): true}, 65)
			symbol := "NIFTY-20260127-22600-CE"
			if err := e.advance(marketTime(9, 15)); err != nil {
				t.Fatal(err)
			}
			if _, err := e.PlaceOrder(symbol, lots); err != nil {
				t.Fatal(err)
			}
			if err := e.advance(marketTime(9, 16)); err != nil {
				t.Fatal(err)
			}
			position := e.Positions[0]
			want := (models.Price(7777777) - 10000) * models.Price(lots) * 65
			if len(position.TotalPnL) != 1 || position.TotalPnL[0].PnL != want || !position.TotalPnL[0].Timestamp.Equal(marketTime(9, 16)) {
				t.Fatalf("incorrect completed-candle UnrealizedPnL: %+v", position.TotalPnL)
			}
			if err := e.advance(marketTime(9, 17)); err != nil {
				t.Fatal(err)
			}
			if len(position.TotalPnL) != 1 || position.Candle != nil {
				t.Fatal("missing candle appended UnrealizedPnL or reused stale data")
			}
			if err := e.advance(marketTime(9, 18)); err != nil {
				t.Fatal(err)
			}
			if len(position.TotalPnL) != 2 || !position.TotalPnL[1].Timestamp.Equal(marketTime(9, 18)) {
				t.Fatal("UnrealizedPnL did not resume after missing candle")
			}
			if _, err := e.PlaceOrder(symbol, -lots); err != nil {
				t.Fatal(err)
			}
			if err := e.advance(marketTime(9, 19)); err != nil {
				t.Fatal(err)
			}
			if position.Quantity != 0 || position.UnrealizedPnL != 0 || len(position.TotalPnL) != 3 || position.TotalPnL[2].PnL != position.RealizedPnL {
				t.Fatal("closed position must record realized P&L and reset unrealized P&L")
			}
			if err := e.advance(marketTime(9, 20)); err != nil {
				t.Fatal(err)
			}
			if len(position.TotalPnL) != 4 || position.TotalPnL[3].PnL != position.RealizedPnL {
				t.Fatal("closed position must retain realized P&L on later ticks")
			}

		})
	}
}

func TestMissingContractLeavesOrderPending(t *testing.T) {
	e := executionFixture(t, nil)
	if err := e.advance(marketTime(9, 15)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.PlaceOrder("NIFTY-20260127-99999-CE", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.PlaceOrder("NIFTY-20260127-22600-CE", 1); err != nil {
		t.Fatal(err)
	}
	for _, minute := range []int{16, 17} {
		if err := e.advance(marketTime(9, minute)); err != nil {
			t.Fatal(err)
		}
		if e.Orders[0].Status != models.OrderPending || e.Orders[1].Status != models.OrderFilled || len(e.Positions) != 1 || len(e.Positions[0].Trades) != 1 {
			t.Fatal("missing contract blocked another order or created a duplicate fill")
		}
	}
}
