package selector

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/models"
	"optionsbacktest/pricing"
)

func TestCalculateContractDelta(t *testing.T) {
	for _, kind := range []models.OptionType{models.Call, models.Put} {
		t.Run(string(kind), func(t *testing.T) {
			repo, mock := selectorRepository(t)
			at := time.Date(2026, 1, 27, 9, 30, 0, 0, marketLocation)
			contract := &models.Option{ID: 101, StrikePrice: 2250000, Expiry: time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC), OptionType: kind}
			// Same-day expiry leaves six hours, not a negative interval to midnight.
			years := 6.0 / (24 * pricing.DaysPerYear)
			premium, err := pricing.BlackScholesPrice(22500, 22500, years, 0.2, kind)
			if err != nil {
				t.Fatal(err)
			}
			paise := int64(math.Round(premium * 100))
			mock.ExpectQuery("FROM lab.option_candles").WithArgs(int64(101), "5m", at, at).
				WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).
					AddRow(at.UTC(), "5m", paise, paise, paise, paise+10000, 0)).RowsWillBeClosed()
			got, err := calculateContractDelta(context.Background(), contract, &models.Candle{Timestamp: at, Open: 2250000, Close: 2400000}, repo)
			want, pricingErr := pricing.DeltaFromPremium(22500, 22500, float64(paise)/100, years, kind)
			if err != nil || pricingErr != nil || math.Abs(got-want) > 1e-12 {
				t.Fatalf("delta=%g want=%g error=%v pricing error=%v", got, want, err, pricingErr)
			}
		})
	}
}

func TestCalculateContractDeltaRequiresInputs(t *testing.T) {
	for _, test := range []struct {
		contract *models.Option
		candle   *models.Candle
	}{
		{}, {contract: &models.Option{}}, {candle: &models.Candle{}},
	} {
		if _, err := calculateContractDelta(context.Background(), test.contract, test.candle, nil); err == nil {
			t.Fatal("expected missing input error")
		}
	}
}

func TestCalculateContractDeltaRequiresExactCandle(t *testing.T) {
	at := time.Date(2026, 1, 1, 9, 30, 0, 0, marketLocation)
	for _, test := range []struct {
		name      string
		timestamp time.Time
	}{
		{"missing", time.Time{}}, {"previous", at.Add(-5 * time.Minute)}, {"future", at.Add(5 * time.Minute)},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := selectorRepository(t)
			rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
			if !test.timestamp.IsZero() {
				rows.AddRow(test.timestamp, "5m", 50000, 50000, 50000, 50000, 0)
			}
			mock.ExpectQuery("FROM lab.option_candles").WithArgs(int64(101), "5m", at, at).WillReturnRows(rows).RowsWillBeClosed()
			_, err := calculateContractDelta(context.Background(), &models.Option{ID: 101}, &models.Candle{Timestamp: at}, repo)
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected missing exact candle, got %v", err)
			}
		})
	}
}

func TestCalculateContractDeltaPropagatesErrors(t *testing.T) {
	for _, stage := range []string{"repository", "expiry", "pricing"} {
		t.Run(stage, func(t *testing.T) {
			repo, mock := selectorRepository(t)
			at := time.Date(2026, 1, 1, 9, 30, 0, 0, marketLocation)
			contract := &models.Option{ID: 101, StrikePrice: 2250000, OptionType: models.Call, Expiry: at.AddDate(0, 0, 27)}
			failure := errors.New("database unavailable")
			query := mock.ExpectQuery("FROM lab.option_candles").WithArgs(int64(101), "5m", at, at)
			if stage == "repository" {
				query.WillReturnError(failure)
			} else {
				if stage == "expiry" {
					contract.Expiry = at.AddDate(0, 0, -1)
				}
				query.WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).AddRow(at, "5m", -1, 0, 0, 0, 0)).RowsWillBeClosed()
			}
			_, err := calculateContractDelta(context.Background(), contract, &models.Candle{Timestamp: at, Open: 2250000}, repo)
			if err == nil {
				t.Fatal("expected calculation error")
			}
			if stage == "repository" && !errors.Is(err, failure) {
				t.Fatalf("lost repository error: %v", err)
			}
			if stage == "expiry" && !strings.Contains(err.Error(), "time to expiry") {
				t.Fatalf("lost time error: %v", err)
			}
			if stage == "pricing" && !strings.Contains(err.Error(), "delta from premium") {
				t.Fatalf("lost pricing error: %v", err)
			}
		})
	}
}

func TestCalculateContractDeltaRespectsCancellation(t *testing.T) {
	repo, _ := selectorRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := calculateContractDelta(ctx, &models.Option{ID: 101}, &models.Candle{}, repo)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
