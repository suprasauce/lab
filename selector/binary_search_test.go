package selector

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/models"
	"optionsbacktest/pricing"
)

func TestBinarySearchContractList(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    models.OptionType
		target  float64
		missing map[int]bool
		invalid map[int]bool
		probes  []int
		want    int
	}{
		{"CALL nearest", models.Call, .54, nil, nil, []int{0, 4, 2, 3, 0, 3, 1, 2, 2, 3}, 3},
		{"PUT nearest", models.Put, .46, nil, nil, []int{0, 4, 2, 3, 0, 2, 1, 2, 1, 2}, 1},
		{"missing low", models.Call, .54, map[int]bool{0: true}, nil, []int{0, 1, 4, 2, 3, 1, 3, 2, 3, 1, 3}, 3},
		{"missing high", models.Call, .54, map[int]bool{4: true}, nil, []int{0, 4, 0, 3, 1, 2, 2, 3}, 3},
		{"missing mid", models.Call, .54, map[int]bool{2: true}, nil, []int{0, 4, 2, 1, 3, 0, 3, 1, 2, 1, 3, 0, 3}, 3},
		{"invalid mid premium", models.Call, .54, nil, map[int]bool{2: true}, []int{0, 4, 2, 1, 3, 0, 3, 1, 2, 1, 3, 0, 3}, 3},
		{"single valid delta", models.Call, .54, map[int]bool{0: true, 1: true, 3: true, 4: true}, nil, []int{0, 1, 2, 4, 2, 3}, 2},
		{"below range", models.Call, .01, nil, nil, []int{0, 4, 2, 3, 0, 2, 1, 2, 0, 1}, 0},
		{"above range", models.Put, .99, nil, nil, []int{0, 4, 2, 3, 3, 4}, 4},
		{"no valid delta", models.Call, .54, map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true}, nil, []int{0, 1, 2, 3, 4}, -1},
		{"empty list", models.Call, .54, nil, nil, nil, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := selectorRepository(t)
			expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
			at := time.Date(2026, 1, 27, 15, 30, 0, 0, marketLocation).AddDate(0, 0, -45)
			var contracts []*models.Option
			if test.name != "empty list" {
				for index := 0; index < 5; index++ {
					offset := index
					if test.kind == models.Call {
						offset = 4 - index
					}
					contracts = append(contracts, &models.Option{ID: int64(index + 1), UnderlyingID: 7, StrikePrice: models.Price(2250000 + offset*5000), Expiry: expiry, OptionType: test.kind})
				}
			}
			strikes := make([]models.Price, len(contracts))
			for i, contract := range contracts {
				strikes[i] = contract.StrikePrice
			}
			for _, index := range test.probes {
				contract := contracts[index]
				mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, int64(contract.StrikePrice), string(test.kind)).
					WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type", "synced"}).
						AddRow(contract.ID, 7, 9, expiry, int64(contract.StrikePrice), string(test.kind), true)).RowsWillBeClosed()
				rows := sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"})
				if !test.missing[index] {
					premium, err := pricing.BlackScholesPrice(22500, float64(contract.StrikePrice)/100, 45.0/pricing.DaysPerYear, .2, test.kind)
					if err != nil {
						t.Fatal(err)
					}
					paise := int64(math.Round(premium * 100))
					if test.invalid[index] {
						paise = -1
					}
					rows.AddRow(at, "5m", paise, 0, 0, 0, 0)
				}
				mock.ExpectQuery("FROM lab.option_candles").WithArgs(contract.ID, "5m", at, at).WillReturnRows(rows).RowsWillBeClosed()
			}
			strike, err := NewDeltaSelector(test.kind, test.target).binarySearch(context.Background(), 7, strikes, &models.Candle{Timestamp: at, Open: 2250000}, expiry, repo)
			if test.want == -1 {
				if err == nil || strike != 0 {
					t.Fatalf("expected no valid delta error, got %v %v", strike, err)
				}
			} else if err != nil || strike != strikes[test.want] {
				t.Fatalf("strike=%v expected index=%d err=%v", strike, test.want, err)
			}
		})
	}
}

func TestBinarySearchPropagatesDatabaseError(t *testing.T) {
	repo, mock := selectorRepository(t)
	failure := errors.New("database unavailable")
	mock.ExpectQuery("FROM lab.option_contracts").WillReturnError(failure)
	_, err := NewDeltaSelector(models.Call, .2).binarySearch(context.Background(), 7, []models.Price{2250000}, &models.Candle{}, time.Time{}, repo)
	if !errors.Is(err, failure) {
		t.Fatalf("lost database error: %v", err)
	}
}
