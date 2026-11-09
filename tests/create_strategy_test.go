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

	"optionsbacktest/repository"
)

func strategyRequest(t *testing.T, server *httptest.Server, method, userID, body string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+"/api/strategies", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if userID != "" {
		request.Header.Set("X-User-ID", userID)
	}
	return request
}

func TestCreateStrategyCreated(t *testing.T) {
	mock, server := setupAPI(t)
	// Saving arbitrary source must not compile or execute it.
	script := "def broken(\n# user's unfinished strategy"
	body, err := json.Marshal(map[string]string{"name": "Short Strangle", "script": script})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(repository.QueryCreateStrategy).
		WithArgs(int64(1), "Short Strangle", script, "").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123))).RowsWillBeClosed()
	response, responseBody := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", string(body)))
	if response.StatusCode != http.StatusCreated || responseBody != `{"id":"123"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
}

func TestCreateStrategyWithDescription(t *testing.T) {
	mock, server := setupAPI(t)
	description := "Sell OTM call and put options to collect premium."
	mock.ExpectQuery(repository.QueryCreateStrategy).
		WithArgs(int64(1), "Short Strangle", "pass", description).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123))).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Strangle","script":"pass","description":"Sell OTM call and put options to collect premium."}`))
	if response.StatusCode != http.StatusCreated || body != `{"id":"123"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyWithEmptyDescription(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryCreateStrategy).
		WithArgs(int64(1), "Short Call", "pass", "").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123))).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call","script":"pass","description":""}`))
	if response.StatusCode != http.StatusCreated || body != `{"id":"123"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyInvalidDescriptionType(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call","script":"pass","description":123}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyMissingName(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyEmptyName(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyMissingScript(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyEmptyScript(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call","script":""}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyWrongFieldType(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call","script":123}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyMalformedJSON(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyMissingUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "", `{"name":"Short Call","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyInvalidUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "user-1", `{"name":"Short Call","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyBodyCannotOverrideUserID(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryCreateStrategy).
		WithArgs(int64(1), "Short Call", "pass", "").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123))).RowsWillBeClosed()
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call","script":"pass","user_id":999}`))
	if response.StatusCode != http.StatusCreated || body != `{"id":"123"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyDatabaseFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryCreateStrategy).WithArgs(int64(1), "Short Call", "pass", "").
		WillReturnError(errors.New("private database detail"))
	response, body := sendRequest(t, server, strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call","script":"pass"}`))
	if response.StatusCode != http.StatusInternalServerError || body != `{"error":"Failed to create strategy."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestCreateStrategyRespectsRequestCancellation(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryCreateStrategy).WithArgs(int64(1), "Short Call", "pass", "").
		WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123)))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	request := strategyRequest(t, server, http.MethodPost, "1", `{"name":"Short Call","script":"pass"}`).WithContext(ctx)
	response := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
