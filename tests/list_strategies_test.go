package tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/repository"
)

func TestListStrategiesOrderedWithCompleteScripts(t *testing.T) {
	mock, server := setupAPI(t)
	updated := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	created := updated.Add(-time.Hour)
	older := updated.Add(-time.Minute)
	script := "def on_candle(ctx):\n    pass\n# user's script"
	mock.ExpectQuery(repository.QueryListStrategies).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"}).
			AddRow(3, "Short Strangle", script, created, updated).
			AddRow(2, "Short Call", "pass", created, updated).
			AddRow(1, "Earlier strategy", "...", created, older)).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "1", ""))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	var payload struct {
		Strategies []map[string]any `json:"strategies"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Strategies) != 3 {
		t.Fatalf("unexpected strategies: %s", body)
	}
	first := payload.Strategies[0]
	if len(first) != 5 || first["id"] != "3" || first["name"] != "Short Strangle" || first["script"] != script || first["created_at"] != created.Format(time.RFC3339) || first["updated_at"] != updated.Format(time.RFC3339) || payload.Strategies[1]["id"] != "2" || payload.Strategies[2]["id"] != "1" {
		t.Fatalf("incorrect response: %s", body)
	}
	if !strings.Contains(repository.QueryListStrategies, "ORDER BY updated_at DESC, id DESC") {
		t.Fatal("query must order by update time and ID descending")
	}
}

func TestListStrategiesEmpty(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListStrategies).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"})).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "1", ""))
	if response.StatusCode != http.StatusOK || body != `{"strategies":[]}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListStrategiesScopesUser(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListStrategies).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"})).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "2", ""))
	if response.StatusCode != http.StatusOK || body != `{"strategies":[]}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	if !strings.Contains(repository.QueryListStrategies, "WHERE user_id = $1") {
		t.Fatal("query must filter by requesting user")
	}
}

func TestListStrategiesMissingUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "", ""))
	if response.StatusCode != http.StatusUnauthorized || body != `{"error":"A valid X-User-ID header is required."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListStrategiesInvalidUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "-1", ""))
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListStrategiesDatabaseFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListStrategies).WithArgs(int64(1)).WillReturnError(errors.New("private database detail"))
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "1", ""))
	if response.StatusCode != http.StatusInternalServerError || body != `{"error":"Failed to retrieve strategies."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListStrategiesScanFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListStrategies).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"}).
			AddRow(1, "Short Call", "pass", "invalid timestamp", time.Now())).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "1", ""))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListStrategiesIterationFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListStrategies).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"}).
			AddRow(1, "Short Call", "pass", time.Now(), time.Now()).RowError(0, errors.New("read failed"))).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodGet, "1", ""))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}
