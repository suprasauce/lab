package selector

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/models"
	"optionsbacktest/pricing"
)

func TestExponentialSearchCrossing(t *testing.T) {
	for _, test := range []struct {
		name      string
		kind      models.OptionType
		direction int
		target    float64
		lastStep  int
	}{
		{"CALL increasing strike", models.Call, 1, 0.2, 64},
		{"CALL decreasing strike", models.Call, -1, 0.8, 32},
		{"PUT increasing strike", models.Put, 1, 0.8, 64},
		{"PUT decreasing strike", models.Put, -1, 0.2, 32},
		{"target not reached", models.Call, 1, 1e-12, 64},
		{"missing candle", models.Call, 1, 0.2, 64},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := selectorRepository(t)
			expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
			at := time.Date(2026, 1, 27, 15, 30, 0, 0, marketLocation).AddDate(0, 0, -45)
			atm := &models.Option{ID: 1, UnderlyingID: 7, Expiry: expiry, StrikePrice: 2250000, OptionType: test.kind}
			for step := 1; step <= test.lastStep; step *= 2 {
				strike := int64(atm.StrikePrice) + int64(test.direction*step)*5000
				mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, strike, string(test.kind)).
					WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}).
						AddRow(strike, 7, 9, expiry, strike, string(test.kind))).RowsWillBeClosed()
				premium, err := pricing.BlackScholesPrice(22500, float64(strike)/100, 45.0/pricing.DaysPerYear, 0.2, test.kind)
				if err != nil {
					t.Fatal(err)
				}
				rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
				if test.name != "missing candle" || step != 32 {
					rows.AddRow(at, "5m", int64(math.Round(premium*100)), 0, 0, 0, 0)
				}
				mock.ExpectQuery("FROM lab.option_candles").WithArgs(strike, "5m", at, at).WillReturnRows(rows).RowsWillBeClosed()
			}
			lower, upper, err := NewDeltaSelector(test.kind, test.target).exponentialSearch(context.Background(), 7, &models.Candle{Timestamp: at, Open: 2250000}, expiry, repo, test.direction, atm)
			if test.name == "target not reached" {
				if err == nil || !strings.Contains(err.Error(), "not reached") {
					t.Fatalf("expected limit error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			first, second := test.direction*test.lastStep/2, test.direction*test.lastStep
			if test.name == "missing candle" {
				first = 16 // The missing candidate must not replace the previous valid one.
			}
			if first > second {
				first, second = second, first
			}
			if lower.StrikePrice != atm.StrikePrice+models.Price(first*5000) || upper.StrikePrice != atm.StrikePrice+models.Price(second*5000) {
				t.Fatalf("wrong bracket: lower=%d upper=%d", lower.StrikePrice, upper.StrikePrice)
			}
		})
	}
}
