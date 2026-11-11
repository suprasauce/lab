package selector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/models"
)

func TestDeltaSelectorBuildsContractList(t *testing.T) {
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
		{"missing contract", models.Call, 0.2, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "missing contract" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `{"status":"ingested","contract_id":1}`)
				}))
				t.Cleanup(server.Close)
				t.Setenv("DATA_FETCHER_URL", server.URL)
			}
			repo, mock := selectorRepository(t)
			expiry := time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)
			at := time.Date(2026, 1, 1, 9, 30, 0, 0, marketLocation)
			for i := 0; i < 64; i++ {
				strike := int64(2250000 + test.direction*i*5000)
				if test.name == "missing contract" && i == 2 {
					mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, strike, string(test.kind)).WillReturnError(sql.ErrNoRows)
					mock.ExpectQuery("FROM lab.underlyings").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"symbol"}).AddRow("NIFTY"))
					mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, strike, string(test.kind)).WillReturnError(sql.ErrNoRows)
					continue
				}
				mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), expiry, strike, string(test.kind)).
					WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}).
						AddRow(strike, 7, 9, expiry, strike, string(test.kind))).RowsWillBeClosed()
				if i == 0 {
					mock.ExpectQuery("FROM lab.option_candles").WithArgs(strike, "5m", at, at).
						WillReturnRows(sqlmock.NewRows([]string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}).
							AddRow(at, "5m", 50000, 50000, 50000, 50000, 0)).RowsWillBeClosed()
				}
			}
			firstStrike := int64(2250000)
			if test.target == 0.2 {
				firstStrike += int64(test.direction * 63 * 5000)
			}
			failure := errors.New("stop after list creation")
			mock.ExpectQuery("FROM lab.option_candles").WithArgs(firstStrike, "5m", at, at).WillReturnError(failure)
			contract, err := NewDeltaSelector(test.kind, test.target).Select(context.Background(), 7, &models.Candle{Timestamp: at, Open: 2250000}, expiry, repo)
			if contract != nil || !errors.Is(err, failure) {
				t.Fatalf("expected completed list followed by search error, got contract=%v error=%v", contract, err)
			}
		})
	}
}
