package tests

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"optionsbacktest/controller"
	"optionsbacktest/repository"
	"optionsbacktest/worker"
)

func TestCreateBacktest(t *testing.T) {
	const script = "def initialize(ctx):\n    pass\n# user's script"
	body, err := json.Marshal(map[string]string{"script": script})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, userID, body string
		databaseError      error
		status, calls      int
	}{
		{"accepted", "1", string(body), nil, http.StatusAccepted, 1},
		{"missing user", "", string(body), nil, http.StatusBadRequest, 0},
		{"invalid user", "user-123", string(body), nil, http.StatusBadRequest, 0},
		{"zero user", "0", string(body), nil, http.StatusBadRequest, 0},
		{"negative user", "-1", string(body), nil, http.StatusBadRequest, 0},
		{"overflow user", "9223372036854775808", string(body), nil, http.StatusBadRequest, 0},
		{"missing script", "1", `{}`, nil, http.StatusBadRequest, 0},
		{"empty script", "1", `{"script":""}`, nil, http.StatusBadRequest, 0},
		{"wrong script type", "1", `{"script":12}`, nil, http.StatusBadRequest, 0},
		{"invalid JSON", "1", `{`, nil, http.StatusBadRequest, 0},
		{"unknown user", "1", string(body), &pgconn.PgError{Code: "23503", ConstraintName: "backtests_user_id_fkey"}, http.StatusInternalServerError, 1},
		{"database failure", "1", string(body), errors.New("private database detail"), http.StatusInternalServerError, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			db := sql.OpenDB(backtestConnector{query: func(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
				calls++
				if !strings.Contains(query, "INSERT INTO lab.backtests (user_id, script)") || !strings.Contains(query, "VALUES ($1, $2)") || !strings.Contains(query, "RETURNING id") {
					t.Fatalf("unexpected insert: %s", query)
				}
				if len(args) != 2 || args[0].Value != int64(1) || args[1].Value != script {
					t.Fatalf("request was not passed as SQL parameters: %+v", args)
				}
				if ctx.Value(requestContextKey{}) != "request" {
					t.Fatal("request context did not reach the repository")
				}
				if tt.databaseError != nil {
					return nil, tt.databaseError
				}
				return &backtestRows{}, nil
			}})
			t.Cleanup(func() { db.Close() })
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/backtests", strings.NewReader(tt.body))
			ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), requestContextKey{}, "request"))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Request.Header.Set("X-User-ID", tt.userID)
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

			pool := worker.NewWorkerPool()
			controller.CreateBacktest(ctx, repository.NewRepository(db, logger), pool, logger)

			if recorder.Code != tt.status || calls != tt.calls {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, calls, recorder.Body)
			}
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if tt.status == http.StatusAccepted {
				if len(response) != 1 || response["id"] != "123" {
					t.Fatalf("unexpected response: %v", response)
				}
			} else if response["error"] == nil {
				t.Fatal("error response missing error message")
			}
			if strings.Contains(recorder.Body.String(), "private database detail") {
				t.Fatal("database details leaked into the response")
			}
		})
	}
}

type requestContextKey struct{}

type backtestConnector struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c backtestConnector) Connect(context.Context) (driver.Conn, error) {
	return backtestConnection{c}, nil
}
func (backtestConnector) Driver() driver.Driver { return backtestDriver{} }

type backtestDriver struct{}

func (backtestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type backtestConnection struct{ backtestConnector }

func (c backtestConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}
func (backtestConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (backtestConnection) Close() error { return nil }
func (backtestConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type backtestRows struct{ done bool }

func (*backtestRows) Columns() []string { return []string{"id"} }
func (*backtestRows) Close() error      { return nil }
func (r *backtestRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = int64(123)
	return nil
}
