package tests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/repository"
)

func listBacktestsRequest(t *testing.T, server *httptest.Server, userID string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/backtests", nil)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "" {
		request.Header.Set("X-User-ID", userID)
	}
	return request
}

func TestListBacktestsNewestFirst(t *testing.T) {
	mock, server := setupAPI(t)
	newest := time.Date(2026, 11, 7, 10, 15, 0, 0, time.UTC)
	older := newest.Add(-15 * time.Minute)
	mock.ExpectQuery(repository.QueryListBacktests).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "created_at"}).
			AddRow(124, "PENDING", newest).AddRow(123, "COMPLETED", older)).RowsWillBeClosed()
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, "1"))
	want := `{"backtests":[{"id":"124","status":"PENDING","created_at":"2026-11-07T10:15:00Z"},{"id":"123","status":"COMPLETED","created_at":"2026-11-07T10:00:00Z"}]}`
	if response.StatusCode != http.StatusOK || body != want {
		t.Fatalf("status=%d body=%s; want %s", response.StatusCode, body, want)
	}
	if !strings.Contains(repository.QueryListBacktests, "ORDER BY created_at DESC") {
		t.Fatal("list query must order newest first")
	}
}

func TestListBacktestsEmpty(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListBacktests).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "created_at"})).RowsWillBeClosed()
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, "1"))
	if response.StatusCode != http.StatusOK || body != `{"backtests":[]}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListBacktestsFiltersUser(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListBacktests).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "created_at"})).RowsWillBeClosed()
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, "2"))
	if response.StatusCode != http.StatusOK || body != `{"backtests":[]}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	if !strings.Contains(repository.QueryListBacktests, "WHERE user_id = $1") {
		t.Fatal("list query must filter by user")
	}
}

func TestListBacktestsMissingUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, ""))
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListBacktestsInvalidUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, "-1"))
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListBacktestsDatabaseFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListBacktests).WithArgs(int64(1)).WillReturnError(errors.New("private database details"))
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, "1"))
	if response.StatusCode != http.StatusInternalServerError || body != `{"error":"Failed to retrieve backtests."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListBacktestsScanFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListBacktests).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "created_at"}).AddRow(123, "PENDING", "invalid timestamp")).RowsWillBeClosed()
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, "1"))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListBacktestsIterationFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListBacktests).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "created_at"}).
			AddRow(123, "PENDING", time.Now()).RowError(0, errors.New("read failed"))).RowsWillBeClosed()
	response, body := sendRequest(t, server, listBacktestsRequest(t, server, "1"))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}
