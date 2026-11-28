package optionchain

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"optionsbacktest/repository"
)

func TestOffsetAtSessionOpenUsesCurrentOpen(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Date(2026, 2, 3, 9, 15, 0, 0, marketLocation)
	expiry := time.Date(2026, 2, 3, 0, 0, 0, 0, marketLocation)
	for _, kind := range []string{"CE", "PE"} {
		mock.ExpectQuery("SELECT e.id, e.option_chain_fetched").WithArgs("NIFTY", expiry).WillReturnRows(sqlmock.NewRows([]string{"id", "option_chain_fetched"}).AddRow(1, true))
		mock.ExpectQuery("FROM lab.underlyings").WithArgs("NIFTY").WillReturnRows(sqlmock.NewRows([]string{"id", "symbol"}).AddRow(1, "NIFTY"))
		// February 3's Open rounds to 26300, while Close and the previous session differ.
		mock.ExpectQuery("FROM lab.underlying_candles").WithArgs(int64(1), "1m", at).WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).AddRow(at, "1m", 2630805, 2634120, 2578175, 2578445, 0))
		optionType := "CALL"
		if kind == "PE" {
			optionType = "PUT"
		}
		want := "NIFTY-20260203-26300-" + kind
		mock.ExpectQuery("SELECT c.tradingsymbol").WithArgs(int64(1), expiry, int64(2630000), optionType).WillReturnRows(sqlmock.NewRows([]string{"tradingsymbol"}).AddRow(want))
		service := NewOptionChainService(repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
		got, err := service.GetSymbol(context.Background(), "NIFTY", expiry, kind, OptionFilter{Type: "offset", Value: 0}, at)
		if err != nil || got != want {
			t.Fatalf("got %q error=%v; want %q", got, err, want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionDoesNotUseOtherMinuteWhenCurrentIsMissing(t *testing.T) {
	for _, filter := range []OptionFilter{{Type: "offset", Value: 0}, {Type: "delta", Value: 0.2}} {
		for _, scenario := range []string{"missing", "later", "previous session"} {
			t.Run(filter.Type+"/"+scenario, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				at := time.Date(2026, 2, 3, 9, 15, 0, 0, marketLocation)
				expiry := time.Date(2026, 2, 24, 0, 0, 0, 0, marketLocation)
				mock.ExpectQuery("SELECT e.id, e.option_chain_fetched").WithArgs("NIFTY", expiry).WillReturnRows(sqlmock.NewRows([]string{"id", "option_chain_fetched"}).AddRow(1, true))
				mock.ExpectQuery("FROM lab.underlyings").WithArgs("NIFTY").WillReturnRows(sqlmock.NewRows([]string{"id", "symbol"}).AddRow(1, "NIFTY"))
				rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
				if scenario != "missing" {
					stamp := at.Add(time.Minute)
					if scenario == "previous session" {
						stamp = at.Add(-24 * time.Hour)
					}
					rows.AddRow(stamp, "1m", 2500000, 2501000, 2499000, 2500000, 0)
				}
				mock.ExpectQuery("FROM lab.underlying_candles").WithArgs(int64(1), "1m", at).WillReturnRows(rows)
				service := NewOptionChainService(repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
				got, err := service.GetSymbol(context.Background(), "NIFTY", expiry, "CE", filter, at)
				if err != nil || got != "" {
					t.Fatalf("used unavailable current-minute data: %q %v", got, err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
