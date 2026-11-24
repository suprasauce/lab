package repository

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"optionsbacktest/models"
)

func TestOptionContractDatabaseOnly(t *testing.T) {
	for _, bySymbol := range []bool{false, true} {
		for _, outcome := range []string{"found", "missing", "database error"} {
			name := outcome
			if bySymbol {
				name += "/symbol"
			}
			t.Run(name, func(t *testing.T) {
				db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				repo := NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
				expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
				var expectation *sqlmock.ExpectedQuery
				if bySymbol {
					expectation = mock.ExpectQuery(queryGetOptionContractByTradingSymbol).WithArgs("NIFTY-20260127-26000-CE")
				} else {
					expectation = mock.ExpectQuery(queryGetOptionContract).WithArgs(int64(7), expiry, int64(2600000), "CALL")
				}
				rows := sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry", "strike_price", "option_type", "underlying_symbol", "lot_size"})
				failure := errors.New("database unavailable")
				switch outcome {
				case "found":
					expectation.WillReturnRows(rows.AddRow(101, 7, 9, expiry, 2600000, "CALL", "NIFTY", 65))
				case "missing":
					expectation.WillReturnRows(rows)
				default:
					expectation.WillReturnError(failure)
				}
				var contract models.Option
				if bySymbol {
					contract, err = repo.GetOptionContractByTradingSymbol(context.Background(), "NIFTY-20260127-26000-CE")
				} else {
					contract, err = repo.GetOptionContract(context.Background(), 7, expiry, 2600000, models.Call)
				}
				switch outcome {
				case "found":
					if err != nil || contract.ID != 101 || contract.LotSize != 65 || contract.StrikePrice != 2600000 {
						t.Fatalf("contract=%+v error=%v", contract, err)
					}
				case "missing":
					if !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("error=%v", err)
					}
				default:
					if !errors.Is(err, failure) {
						t.Fatalf("error=%v", err)
					}
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestEnsureOptionChainDatabaseOnly(t *testing.T) {
	for _, outcome := range []string{"available", "incomplete", "missing", "database error"} {
		t.Run(outcome, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
			query := mock.ExpectQuery(queryGetOptionChainStatus).WithArgs("NIFTY", expiry)
			rows := sqlmock.NewRows([]string{"id", "option_chain_fetched"})
			failure := errors.New("database unavailable")
			switch outcome {
			case "available":
				query.WillReturnRows(rows.AddRow(9, true))
			case "incomplete":
				query.WillReturnRows(rows.AddRow(9, false))
			case "missing":
				query.WillReturnRows(rows)
			default:
				query.WillReturnError(failure)
			}
			err = repo.EnsureOptionChain(context.Background(), "NIFTY", expiry)
			if outcome == "available" && err != nil {
				t.Fatal(err)
			}
			if (outcome == "incomplete" || outcome == "missing") && (err == nil || !strings.Contains(err.Error(), "option chain not available")) {
				t.Fatalf("error=%v", err)
			}
			if outcome == "database error" && !errors.Is(err, failure) {
				t.Fatalf("error=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
