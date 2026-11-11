package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/models"
)

func TestOptionContractForDay(t *testing.T) {
	for _, test := range []struct {
		name                             string
		exists, synced, dayExists, fetch bool
		status                           string
	}{
		{"fully synced", true, true, false, false, ""},
		{"day already stored", true, false, true, false, ""},
		{"unsynced needs day", true, false, false, true, "ingested"},
		{"missing contract", false, false, false, true, "ingested"},
		{"no candles", false, false, false, true, "no_data"},
		{"fetch failure", false, false, false, true, "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Underlying string      `json:"underlying"`
					Expiry     string      `json:"expiry"`
					Strike     json.Number `json:"strike"`
					OptionType string      `json:"option_type"`
					Day        string      `json:"day"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != "/options/fetch-day" || body.Day != "2025-12-02" || body.Strike != "25000.05" || body.Underlying != "NIFTY" || body.OptionType != "CALL" || body.Expiry != "2025-12-30" {
					t.Errorf("wrong day request: %s %+v", r.URL.Path, body)
				}
				if test.status == "failed" {
					w.WriteHeader(502)
					io.WriteString(w, `{"error":"Breeze unavailable"}`)
					return
				}
				fmt.Fprintf(w, `{"status":%q,"contract_id":101}`, test.status)
			}))
			defer server.Close()
			t.Setenv("DATA_FETCHER_URL", server.URL)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				mock.ExpectClose()
				db.Close()
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Error(err)
				}
			})
			repo := NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
			rows := sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type", "synced"})
			if test.exists {
				rows.AddRow(101, 7, 9, testExpiry, 2500005, "CALL", test.synced)
			}
			mock.ExpectQuery("FROM lab.option_contracts").WithArgs(int64(7), testExpiry, int64(2500005), "CALL").WillReturnRows(rows)
			day := time.Date(2025, 12, 1, 23, 0, 0, 0, time.UTC) // December 2 in IST.
			start := time.Date(2025, 12, 2, 0, 0, 0, 0, optionMarketLocation)
			if test.exists && !test.synced {
				mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(101), start, start.AddDate(0, 0, 1)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(test.dayExists))
			}
			if test.fetch {
				mock.ExpectQuery("FROM lab.underlyings").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"symbol"}).AddRow("NIFTY"))
				if test.status == "ingested" {
					mock.ExpectQuery("FROM lab.option_contracts").WillReturnRows(sqlmock.NewRows([]string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type", "synced"}).AddRow(101, 7, 9, testExpiry, 2500005, "CALL", false))
				}
			}
			contract, err := repo.GetOptionContractForDay(context.Background(), 7, testExpiry, 2500005, models.Call, day)
			if test.status == "no_data" {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("missing candle error lost: %v", err)
				}
			} else if test.status == "failed" {
				if err == nil {
					t.Fatal("fetch failure ignored")
				}
			} else if err != nil || contract.ID != 101 || contract.Synced != test.synced {
				t.Fatalf("contract=%+v err=%v", contract, err)
			}
			if (calls == 1) != test.fetch {
				t.Fatalf("calls=%d fetch=%v", calls, test.fetch)
			}
		})
	}
}
