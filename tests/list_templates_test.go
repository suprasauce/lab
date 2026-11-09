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

func templatesRequest(t *testing.T, server *httptest.Server) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/templates", nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestListTemplatesOrderedWithCompleteScripts(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "7")
	mock, server := setupAPI(t)
	created := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)
	script := "def on_candle(ctx):\n    pass\n# template's full source"
	mock.ExpectQuery(repository.QueryListTemplates).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"}).
			AddRow(2, "Short Call", script, created, updated).
			AddRow(3, "Short Call", "pass", created, updated).
			AddRow(1, "Short Strangle", "...", created, updated)).RowsWillBeClosed()
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	var payload map[string][]map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	templates := payload["templates"]
	if len(payload) != 1 || len(templates) != 3 {
		t.Fatalf("unexpected response: %s", body)
	}
	first := templates[0]
	if len(first) != 5 || first["id"] != "2" || first["name"] != "Short Call" || first["script"] != script || first["created_at"] != created.Format(time.RFC3339) || first["updated_at"] != updated.Format(time.RFC3339) || templates[1]["id"] != "3" || templates[2]["id"] != "1" {
		t.Fatalf("incorrect templates: %s", body)
	}
	if !strings.Contains(repository.QueryListTemplates, "WHERE user_id = $1") || !strings.Contains(repository.QueryListTemplates, "ORDER BY name ASC, id ASC") {
		t.Fatal("template query must filter by owner and order by name and ID")
	}
}

func TestListTemplatesEmpty(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "7")
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListTemplates).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"})).RowsWillBeClosed()
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusOK || body != `{"templates":[]}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesIgnoresCallerIdentity(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "7")
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListTemplates).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"})).RowsWillBeClosed()
	request := templatesRequest(t, server)
	request.Header.Set("X-User-ID", "not-a-user")
	request.URL.RawQuery = "user_id=999&template_owner_user_id=999"
	response, body := sendRequest(t, server, request)
	if response.StatusCode != http.StatusOK || body != `{"templates":[]}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesMissingOwnerConfiguration(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "")
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusInternalServerError || body != `{"error":"Failed to retrieve templates."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesInvalidOwnerConfiguration(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "not-a-user")
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusInternalServerError || body != `{"error":"Failed to retrieve templates."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesNonpositiveOwnerConfiguration(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "0")
	_, server := setupAPI(t)
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesDatabaseFailure(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "7")
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListTemplates).WithArgs(int64(7)).WillReturnError(errors.New("private database detail"))
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusInternalServerError || body != `{"error":"Failed to retrieve templates."}` {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesScanFailure(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "7")
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListTemplates).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"}).
			AddRow(1, "Short Call", "pass", "invalid timestamp", time.Now())).RowsWillBeClosed()
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesIterationFailure(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "7")
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListTemplates).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"}).
			AddRow(1, "Short Call", "pass", time.Now(), time.Now()).RowError(0, errors.New("read failed"))).RowsWillBeClosed()
	response, body := sendRequest(t, server, templatesRequest(t, server))
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
}

func TestListTemplatesRespectsRequestCancellation(t *testing.T) {
	t.Setenv("TEMPLATE_OWNER_USER_ID", "7")
	mock, server := setupAPI(t)
	mock.ExpectQuery(repository.QueryListTemplates).WithArgs(int64(7)).WillDelayFor(time.Second).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "script", "created_at", "updated_at"}))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	request := templatesRequest(t, server).WithContext(ctx)
	response := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
