package optionchain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"optionsbacktest/models"
	"optionsbacktest/repository"
)

func TestDeltaSelectionClosestCompletedCandidate(t *testing.T) {
	for _, kind := range []models.OptionType{models.Call, models.Put} {
		for _, scenario := range []string{"closest", "missing", "invalid premium", "database error"} {
			t.Run(string(kind)+"/"+scenario, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				repo := repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
				service := NewOptionChainService(repo)
				expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, marketLocation)
				at := time.Date(2026, 1, 5, 9, 35, 0, 0, marketLocation)
				bar := at.Add(-time.Minute)
				years, _ := TimeToExpiry(at, expiry.Add(15*time.Hour+30*time.Minute))
				optionType := "CE"
				if kind == models.Put {
					optionType = "PE"
				}
				mock.ExpectQuery("SELECT e.id, e.option_chain_fetched").WithArgs("NIFTY", expiry).
					WillReturnRows(sqlmock.NewRows([]string{"id", "option_chain_fetched"}).AddRow(9, true))
				mock.ExpectQuery("FROM lab.underlyings").WithArgs("NIFTY").
					WillReturnRows(sqlmock.NewRows([]string{"id", "symbol"}).AddRow(1, "NIFTY"))
				mock.ExpectQuery("start_time <= .*INTERVAL '1 minute'").WithArgs(int64(1), at).
					WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).AddRow(bar, "1m", 2400000, 2600000, 2300000, 2500000, 0))
				rows := sqlmock.NewRows([]string{"tradingsymbol", "strike_price", "close"})
				want, best := "", math.Inf(1)
				if scenario == "closest" {
					for _, strike := range []float64{24000, 25000, 26000, 27000} {
						premium, err := BlackScholesPrice(25000, strike, years, 0.2, kind)
						if err != nil {
							t.Fatal(err)
						}
						paise := int64(math.Round(premium * 100))
						delta, err := DeltaFromPremium(25000, strike, float64(paise)/100, years, kind)
						if err != nil {
							t.Fatal(err)
						}
						symbol := fmt.Sprintf("NIFTY-20260127-%.0f-%s", strike, optionType)
						rows.AddRow(symbol, int64(strike*100), paise)
						if difference := math.Abs(math.Abs(delta) - 0.2); difference < best {
							want, best = symbol, difference
						}
					}
					// An invalid premium must not prevent selection of another candidate.
					rows.AddRow("invalid", 2500000, -100)
				}
				if scenario == "invalid premium" {
					rows.AddRow("invalid", 2500000, -100)
				}
				query := mock.ExpectQuery("SELECT c.tradingsymbol, c.strike_price, oc.close").WithArgs(int64(1), expiry, string(kind), bar)
				failure := errors.New("database unavailable")
				if scenario == "database error" {
					query.WillReturnError(failure)
				} else {
					query.WillReturnRows(rows)
				}
				got, err := service.GetSymbol(context.Background(), "NIFTY", expiry, optionType, OptionFilter{Type: "delta", Value: 0.2}, at)
				if scenario == "database error" {
					if !errors.Is(err, failure) {
						t.Fatalf("error=%v", err)
					}
				} else if err != nil || got != want {
					t.Fatalf("symbol=%q error=%v; want %q", got, err, want)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestDeltaTargetRange(t *testing.T) {
	for _, target := range []float64{0.09, 1.01, math.NaN(), math.Inf(1)} {
		service := NewOptionChainService(nil)
		if _, err := service.getSymbolByDelta(context.Background(), "NIFTY", time.Time{}, "CE", target, time.Time{}); err == nil {
			t.Fatalf("expected invalid target error for %v", target)
		}
	}
	for _, target := range []float64{0.10, 1.0} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			failure := errors.New("lookup reached")
			mock.ExpectQuery("FROM lab.underlyings").WithArgs("NIFTY").WillReturnError(failure)
			service := NewOptionChainService(repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
			_, err = service.getSymbolByDelta(context.Background(), "NIFTY", time.Time{}, "CE", target, time.Time{})
			if !errors.Is(err, failure) {
				t.Fatalf("valid boundary rejected: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
