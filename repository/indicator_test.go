package repository

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestIndicatorCandlesSharedCache(t *testing.T) {
	for _, symbol := range []string{"NIFTY", "NIFTY-20260127-25000-CE", "EMPTY"} {
		t.Run(symbol, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
			if symbol != "EMPTY" {
				rows.AddRow(time.Now(), "1m", 100, 110, 90, 105, 10)
			}
			mock.ExpectQuery(queryGetIndicatorCandles).WithArgs(symbol).WillReturnRows(rows).RowsWillBeClosed()
			first, err := repo.GetIndicatorCandles(context.Background(), symbol)
			if err != nil {
				t.Fatal(err)
			}
			second, err := repo.GetIndicatorCandles(context.Background(), symbol)
			if err != nil {
				t.Fatal(err)
			}
			if first == nil || second == nil || len(first) != len(second) {
				t.Fatal("expected cached non-nil slice")
			}
			if len(first) > 0 && &first[0] != &second[0] {
				t.Fatal("cache returned a copy of the candle array")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIndicatorCandlesDoesNotCacheErrors(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	failure := errors.New("database unavailable")
	mock.ExpectQuery(queryGetIndicatorCandles).WithArgs("NIFTY").WillReturnError(failure)
	if _, err := repo.GetIndicatorCandles(context.Background(), "NIFTY"); !errors.Is(err, failure) {
		t.Fatalf("unexpected error: %v", err)
	}
	mock.ExpectQuery(queryGetIndicatorCandles).WithArgs("NIFTY").WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}))
	if _, err := repo.GetIndicatorCandles(context.Background(), "NIFTY"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
