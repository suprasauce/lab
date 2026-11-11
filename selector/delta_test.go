package selector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/models"
	"optionsbacktest/pricing"
)

func TestDeltaSelectorBuildsStrikeListLazily(t *testing.T) {
	for _, test := range []struct {
		name      string
		kind      models.OptionType
		target    float64
		direction int
	}{
		{"CALL above ATM", models.Call, 0.2, 1},
		{"CALL below ATM", models.Call, 0.8, -1},
		{"PUT above ATM", models.Put, 0.8, 1},
		{"PUT below ATM", models.Put, 0.2, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := selectorRepository(t)
			expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
			at := time.Date(2026, 1, 1, 9, 30, 0, 0, marketLocation)
			mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, int64(2250000), string(test.kind)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type", "synced"}).
					AddRow(2250000, 7, 9, expiry, 2250000, string(test.kind), true)).RowsWillBeClosed()
			mock.ExpectQuery("FROM lab.option_candles").WithArgs(int64(2250000), "5m", at, at).
				WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).
					AddRow(at, "5m", 50000, 50000, 50000, 50000, 0)).RowsWillBeClosed()
			firstStrike := int64(2250000)
			if test.target == 0.2 {
				firstStrike += int64(test.direction * 39 * 5000)
			}
			failure := errors.New("stop after list creation")
			mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, firstStrike, string(test.kind)).WillReturnError(failure)
			contract, err := NewDeltaSelector(test.kind, test.target).Select(context.Background(), 7, &models.Candle{Timestamp: at, Open: 2250000}, expiry, repo)
			if contract != nil || !errors.Is(err, failure) {
				t.Fatalf("expected completed list followed by search error, got contract=%v error=%v", contract, err)
			}
		})
	}
}

func TestDeltaSelectorFetchesVisitedStrikesAndSyncsOnlyWinner(t *testing.T) {
	dayCalls, fullCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Strike json.Number `json:"strike"`
			Day    string      `json:"day"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/options/fetch-day":
			dayCalls++
			if body.Strike != "24450.00" || body.Day != "2025-12-13" {
				t.Errorf("wrong day candidate: %+v", body)
			}
		case "/options/fetch":
			fullCalls++
			if body.Strike != "23500.00" || body.Day != "" {
				t.Errorf("full ingestion was not limited to winner: %+v", body)
			}
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"ingested","contract_id":101}`)
	}))
	defer server.Close()
	t.Setenv("DATA_FETCHER_URL", server.URL)
	repo, mock := selectorRepository(t)
	expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 1, 27, 15, 30, 0, 0, marketLocation).AddDate(0, 0, -45)
	start := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.FixedZone("IST", 19800))
	contractRows := func(strike int64, synced bool) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type", "synced"}).AddRow(strike, 7, 9, expiry, strike, "CALL", synced)
	}
	premiums := map[int]int64{}
	for _, offset := range []int{0, 39, 20, 19} {
		strike := int64(2250000 + offset*5000)
		price, err := pricing.BlackScholesPrice(22500, float64(strike)/100, 45.0/pricing.DaysPerYear, .2, models.Call)
		if err != nil {
			t.Fatal(err)
		}
		premiums[offset] = int64(math.Round(price * 100))
	}
	// ATM establishes direction. Binary search then visits only the endpoints
	// and two midpoint candidates, rather than resolving all 40 strikes.
	for _, offset := range []int{0, 39, 0, 20, 19} {
		strike := int64(2250000 + offset*5000)
		if offset == 39 {
			mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, strike, "CALL").WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery("FROM lab.underlyings").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"symbol"}).AddRow("NIFTY"))
			mock.ExpectQuery("FROM lab.option_contracts").WillReturnRows(contractRows(strike, false))
		} else {
			mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, strike, "CALL").WillReturnRows(contractRows(strike, false))
			mock.ExpectQuery("SELECT EXISTS").WithArgs(strike, start, start.AddDate(0, 0, 1)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		}
		mock.ExpectQuery("FROM lab.option_candles").WithArgs(strike, "5m", at, at).WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).AddRow(at, "5m", premiums[offset], 0, 0, 0, 0))
	}
	selected := int64(2350000)
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, selected, "CALL").WillReturnRows(contractRows(selected, false))
	mock.ExpectQuery("FROM lab.underlyings").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"symbol"}).AddRow("NIFTY"))
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, selected, "CALL").WillReturnRows(contractRows(selected, true))
	target, err := pricing.DeltaFromPremium(22500, float64(selected)/100, float64(premiums[20])/100, 45.0/pricing.DaysPerYear, models.Call)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := NewDeltaSelector(models.Call, target).Select(context.Background(), 7, &models.Candle{Timestamp: at, Open: 2250000}, expiry, repo)
	if err != nil || contract == nil || contract.StrikePrice != models.Price(selected) || !contract.Synced || dayCalls != 1 || fullCalls != 1 {
		t.Fatalf("contract=%+v err=%v day calls=%d full calls=%d", contract, err, dayCalls, fullCalls)
	}
}

func TestBinarySearchSkipsStrikeWithNoDayData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/options/fetch-day" {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"no_data","contract_id":null}`)
	}))
	defer server.Close()
	t.Setenv("DATA_FETCHER_URL", server.URL)
	repo, mock := selectorRepository(t)
	expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 1, 1, 9, 30, 0, 0, marketLocation)
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, int64(2250000), "CALL").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("FROM lab.underlyings").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"symbol"}).AddRow("NIFTY"))
	mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, int64(2255000), "CALL").
		WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type", "synced"}).AddRow(101, 7, 9, expiry, 2255000, "CALL", true))
	mock.ExpectQuery("FROM lab.option_candles").WithArgs(int64(101), "5m", at, at).
		WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).AddRow(at, "5m", 50000, 0, 0, 0, 0))
	strike, err := NewDeltaSelector(models.Call, .2).binarySearch(context.Background(), 7, []models.Price{2250000, 2255000}, &models.Candle{Timestamp: at, Open: 2250000}, expiry, repo)
	if err != nil || strike != 2255000 {
		t.Fatalf("strike=%d err=%v", strike, err)
	}
}
