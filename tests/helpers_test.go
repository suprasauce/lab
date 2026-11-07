package tests

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"optionsbacktest/repository"
	"optionsbacktest/worker"
)

// Each test gets its own server, SQL mock, repository, logger, and worker queue.
func setupAPI(t *testing.T) (sqlmock.Sqlmock, *httptest.Server) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	repo := repository.NewRepository(db, logger)
	pool := worker.NewWorkerPool(repo, logger)
	t.Cleanup(func() {
		mock.ExpectClose()
		if err := db.Close(); err != nil {
			t.Error(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	server := newTestServer(repo, pool, logger)
	t.Cleanup(server.Close)
	return mock, server
}

func backtestRequest(t *testing.T, server *httptest.Server, userID, body string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/backtests", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if userID != "" {
		request.Header.Set("X-User-ID", userID)
	}
	return request
}

func sendRequest(t *testing.T, server *httptest.Server, request *http.Request) (*http.Response, string) {
	t.Helper()
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(body)
}
