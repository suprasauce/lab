package tests

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/repository"
)

func getBacktestRequest(t *testing.T, server *httptest.Server, id, userID string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/backtests/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "" {
		request.Header.Set("X-User-ID", userID)
	}
	return request
}

func TestGetBacktestPending(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "results", "script"}).AddRow(123, "PENDING", nil, "saved script")).RowsWillBeClosed()
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "1"))
	if response.StatusCode != http.StatusOK || body != `{"id":"123","status":"PENDING","script":"saved script"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestCompleted(t *testing.T) {
	mock, server := setupAPI(t)
	results := `{"summary":{"total_trades":1,"total_pnl":5298.75},"trades":[{"expiry":"2026-01-27","underlying":"NIFTY","strike":26000.00,"option_type":"CALL","side":"SHORT","entry_timestamp":"2025-12-15T09:30:00+05:30","exit_timestamp":"2026-01-06T15:15:00+05:30","entry_price":461.85,"exit_price":391.20,"quantity":75,"pnl":5298.75}]}`
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "results", "script"}).AddRow(123, "COMPLETED", []byte(results), "saved script")).RowsWillBeClosed()
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "1"))
	var actual map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &actual); err != nil {
		t.Fatal(err)
	}
	var expected map[string]json.RawMessage
	if err := json.Unmarshal([]byte(results), &expected); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(actual) != 5 || string(actual["script"]) != `"saved script"` || string(actual["id"]) != `"123"` || string(actual["status"]) != `"COMPLETED"` || string(actual["summary"]) != string(expected["summary"]) || string(actual["trades"]) != string(expected["trades"]) {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestFailed(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "results", "script"}).AddRow(123, "FAILED", []byte(`{"error":"Insufficient historical candle data"}`), "saved script")).RowsWillBeClosed()
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "1"))
	if response.StatusCode != http.StatusOK || body != `{"id":"123","status":"FAILED","script":"saved script","error":"Insufficient historical candle data"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestMissingUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", ""))
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestInvalidUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "user-1"))
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestInvalidID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "invalid", "1"))
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestNotFound(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(1)).WillReturnError(sql.ErrNoRows)
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "1"))
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestOtherUserNotFound(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(2)).WillReturnError(sql.ErrNoRows)
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "2"))
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestDatabaseFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(1)).WillReturnError(errors.New("private database details"))
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "1"))
	if response.StatusCode != http.StatusInternalServerError || strings.Contains(body, "private") {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestInvalidStoredResults(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "results", "script"}).AddRow(123, "COMPLETED", []byte(`{"summary":`), "saved script")).RowsWillBeClosed()
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "1"))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestGetBacktestPreservesExactScript(t *testing.T) {
	mock, server := setupAPI(t)
	script := "# Saved snapshot\r\ndef initialize(ctx):\n    value = \"NIFTY\"\n"
	mock.ExpectQuery(repository.QueryGetBacktestForUser).WithArgs(int64(123), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "results", "script"}).AddRow(123, "PENDING", nil, script)).RowsWillBeClosed()
	response, body := sendRequest(t, server, getBacktestRequest(t, server, "123", "1"))
	var result struct {
		Script string `json:"script"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.Script != script {
		t.Fatalf("status=%d script=%q", response.StatusCode, result.Script)
	}
}
