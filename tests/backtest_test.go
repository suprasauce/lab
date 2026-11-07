package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"

	"optionsbacktest/repository"
)

func TestCreateBacktestAccepted(t *testing.T) {
	mock, server := setupAPI(t)
	script := "def initialize(ctx):\n    pass\n# user's script"
	body, err := json.Marshal(map[string]string{"script": script})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(repository.QueryCreateBacktest).
		WithArgs(int64(1), script).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123))).
		RowsWillBeClosed()

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "1", string(body)))

	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result["id"] != "123" {
		t.Fatalf("unexpected response: %v", result)
	}
}

func TestCreateBacktestMissingUserID(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "", `{"script":"pass"}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request headers") {
		t.Fatalf("expected header validation error: %s", responseBody)
	}
}

func TestCreateBacktestInvalidUserID(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "user-123", `{"script":"pass"}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request headers") {
		t.Fatalf("expected header validation error: %s", responseBody)
	}
}

func TestCreateBacktestZeroUserID(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "0", `{"script":"pass"}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request headers") {
		t.Fatalf("expected header validation error: %s", responseBody)
	}
}

func TestCreateBacktestNegativeUserID(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "-1", `{"script":"pass"}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request headers") {
		t.Fatalf("expected header validation error: %s", responseBody)
	}
}

func TestCreateBacktestOverflowUserID(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "9223372036854775808", `{"script":"pass"}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request headers") {
		t.Fatalf("expected header validation error: %s", responseBody)
	}
}

func TestCreateBacktestMissingScript(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "1", `{}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request body") {
		t.Fatalf("expected body validation error: %s", responseBody)
	}
}

func TestCreateBacktestEmptyScript(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "1", `{"script":""}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request body") {
		t.Fatalf("expected body validation error: %s", responseBody)
	}
}

func TestCreateBacktestInvalidScriptType(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "1", `{"script":12}`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request body") {
		t.Fatalf("expected body validation error: %s", responseBody)
	}
}

func TestCreateBacktestInvalidJSON(t *testing.T) {
	_, server := setupAPI(t)

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "1", `{`))

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if !strings.Contains(responseBody, "invalid request body") {
		t.Fatalf("expected body validation error: %s", responseBody)
	}
}

func TestCreateBacktestUnknownUser(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryCreateBacktest).
		WithArgs(int64(1), "pass").
		WillReturnError(&pgconn.PgError{Code: "23503", ConstraintName: "backtests_user_id_fkey"})

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "1", `{"script":"pass"}`))

	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if responseBody != `{"error":"Backtest job submission failed."}` {
		t.Fatalf("unexpected error response: %s", responseBody)
	}
}

func TestCreateBacktestDatabaseFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryCreateBacktest).
		WithArgs(int64(1), "pass").
		WillReturnError(errors.New("private database detail"))

	response, responseBody := sendRequest(t, server, backtestRequest(t, server, "1", `{"script":"pass"}`))

	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if responseBody != `{"error":"Backtest job submission failed."}` {
		t.Fatalf("database details leaked into response: %s", responseBody)
	}
}

func TestCreateBacktestRespectsRequestCancellation(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryCreateBacktest).
		WithArgs(int64(1), "pass").
		WillDelayFor(time.Second).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123)))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	request := backtestRequest(t, server, "1", `{"script":"pass"}`).WithContext(ctx)

	// Use the test server's handler directly so cancellation does not prevent the test
	// from reading the HTTP response, as it would over a closed client connection.
	response := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("request cancellation did not stop the insert: status=%d body=%s", response.Code, response.Body)
	}
}
