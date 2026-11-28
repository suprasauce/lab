package scripting

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.starlark.net/starlark"
	"optionsbacktest/engine"
	"optionsbacktest/repository"
)

func TestGetLotSize(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 5, 9, 15, 0, 0, location)
	mock.ExpectQuery("FROM lab.underlying_candles").WithArgs(int64(1), "1m", at).WillReturnRows(sqlmock.NewRows(candleColumns()).AddRow(candleRow(at, 2250000)...))
	symbol := "NIFTY-20260127-22600-CE"
	mock.ExpectQuery("FROM lab.underlyings").WithArgs(symbol).WillReturnRows(sqlmock.NewRows([]string{"id", "symbol"}))
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(symbol).WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry", "strike", "option_type", "underlying_symbol", "lot_size"}).AddRow(101, 1, 9, time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC), 2260000, "CALL", "NIFTY", 65))
	mock.ExpectQuery("FROM lab.underlyings").WithArgs("NIFTY").WillReturnRows(sqlmock.NewRows([]string{"id", "symbol"}).AddRow(1, "NIFTY"))
	repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// Lot-size lookup needs no candle data or open position.
	_, err = engine.NewEngine(repo).Simulate(context.Background(), "1", at, at, func(execution *engine.Execution) error {
		if !execution.Time.Equal(at) {
			return nil
		}
		c := &strategyContext{execution: execution}
		_, err := starlark.ExecFile(&starlark.Thread{}, "lot.star", `
def check(ctx):
    if ctx.get_lot_size("NIFTY-20260127-22600-CE") != 65:
        fail("incorrect option lot size")
    if ctx.get_lot_size(tradingsymbol="NIFTY-20260127-22600-CE") != 65:
        fail("cached lot size changed")
    if ctx.get_lot_size("NIFTY") != 1:
        fail("underlying must use units")
check(ctx)
`, starlark.StringDict{"ctx": c})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetLotSizeErrors(t *testing.T) {
	for _, expression := range []string{`ctx.get_lot_size()`, `ctx.get_lot_size(123)`} {
		_, err := starlark.Eval(&starlark.Thread{}, "lot.star", expression, starlark.StringDict{"ctx": positionContext()})
		if err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
	for _, c := range []*strategyContext{{}, {initializing: true, execution: &engine.Execution{}}} {
		_, err := starlark.Eval(&starlark.Thread{}, "lot.star", `ctx.get_lot_size("NIFTY")`, starlark.StringDict{"ctx": c})
		if err == nil {
			t.Fatal("lot size exposed outside callback")
		}
	}
}
