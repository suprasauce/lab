package tests

import (
	"context"
	"database/sql"
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

func updateStrategyRequest(t *testing.T, server *httptest.Server, id, userID, body string) *http.Request {
	t.Helper()
	request := strategyRequest(t, server, http.MethodPut, userID, body)
	request.URL.Path += "/" + id
	return request
}

func TestUpdateStrategySuccess(t *testing.T) {
	mock, server := setupAPI(t)
	script := "def broken(\n# user's updated strategy"
	body, err := json.Marshal(map[string]string{"name": "NIFTY Short Strangle", "script": script})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(repository.QueryUpdateStrategy).
		WithArgs("NIFTY Short Strangle", script, int64(123), int64(1), "").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123))).RowsWillBeClosed()
	response, responseBody := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", string(body)))
	if response.StatusCode != http.StatusOK || responseBody != `{"id":"123"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
}

func TestUpdateStrategyWithDescription(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryUpdateStrategy).
		WithArgs("Updated", "pass", int64(123), int64(1), "Updated description.").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123))).RowsWillBeClosed()
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":"Updated","script":"pass","description":"Updated description."}`))
	if response.StatusCode != http.StatusOK || body != `{"id":"123"}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyInvalidDescriptionType(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":"Updated","script":"pass","description":123}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyQueryPreservesOwnershipAndCreationTime(t *testing.T) {
	// Restrict SET to editable fields and the server-generated modification time.
	query := strings.Join(strings.Fields(repository.QueryUpdateStrategy), " ")
	want := "UPDATE lab.strategies SET name = $1, script = $2, description = $5, updated_at = NOW() WHERE id = $3 AND user_id = $4 RETURNING id;"
	if query != want {
		t.Fatalf("update must preserve owner, ID and created_at while refreshing updated_at: %s", query)
	}
}

func TestUpdateStrategyInvalidID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "invalid", "1", `{"name":"Updated","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyZeroID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "0", "1", `{"name":"Updated","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyOverflowID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "9223372036854775808", "1", `{"name":"Updated","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyMalformedJSON(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyWrongFieldType(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":"Updated","script":123}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyMissingName(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyMissingScript(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":"Updated"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyEmptyRequiredFields(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":"","script":""}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyMissingUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "", `{"name":"Updated","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyInvalidUserID(t *testing.T) {
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "-1", `{"name":"Updated","script":"pass"}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyNotFound(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryUpdateStrategy).WithArgs("Updated", "pass", int64(123), int64(1), "").WillReturnError(sql.ErrNoRows)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":"Updated","script":"pass"}`))
	if response.StatusCode != http.StatusNotFound || body != `{"error":"Strategy not found."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyOtherUserNotFound(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryUpdateStrategy).WithArgs("Updated", "pass", int64(123), int64(2), "").WillReturnError(sql.ErrNoRows)
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "2", `{"name":"Updated","script":"pass","user_id":1}`))
	if response.StatusCode != http.StatusNotFound || body != `{"error":"Strategy not found."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyDatabaseFailure(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryUpdateStrategy).WithArgs("Updated", "pass", int64(123), int64(1), "").WillReturnError(errors.New("private database detail"))
	response, body := sendRequest(t, server, updateStrategyRequest(t, server, "123", "1", `{"name":"Updated","script":"pass"}`))
	if response.StatusCode != http.StatusInternalServerError || body != `{"error":"Failed to update strategy."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestUpdateStrategyRespectsRequestCancellation(t *testing.T) {
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryUpdateStrategy).WithArgs("Updated", "pass", int64(123), int64(1), "").
		WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(123)))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	request := updateStrategyRequest(t, server, "123", "1", `{"name":"Updated","script":"pass"}`).WithContext(ctx)
	response := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
