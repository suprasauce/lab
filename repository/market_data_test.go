package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"optionsbacktest/models"
)

var testExpiry = time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)

type contractDB struct {
	queries, symbols                int
	existing, inserted              bool
	initialErr, afterErr, symbolErr error
	strike                          models.Price
}

func (db *contractDB) query(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch query {
	case queryGetOptionContract:
		db.queries++
		if len(args) != 4 || args[0].Value != int64(7) || !args[1].Value.(time.Time).Equal(testExpiry) || args[2].Value != int64(db.strike) || args[3].Value != "CALL" {
			return nil, fmt.Errorf("unexpected contract parameters: %+v", args)
		}
		if db.queries == 1 && db.initialErr != nil {
			return nil, db.initialErr
		}
		if db.queries > 1 && db.afterErr != nil {
			return nil, db.afterErr
		}
		rows := &contractRows{columns: []string{"id", "underlying_id", "expiry_id", "expiry", "strike_price", "option_type"}}
		if db.existing || (db.queries > 1 && db.inserted) {
			rows.row = []driver.Value{int64(101), int64(7), int64(9), testExpiry, int64(db.strike), "CALL"}
		}
		return rows, nil
	case queryGetUnderlyingSymbol:
		db.symbols++
		if len(args) != 1 || args[0].Value != int64(7) {
			return nil, fmt.Errorf("unexpected underlying parameters")
		}
		if db.symbolErr != nil {
			return nil, db.symbolErr
		}
		return &contractRows{columns: []string{"symbol"}, row: []driver.Value{"NIFTY"}}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func testRepository(t *testing.T, state *contractDB) *MarketDataRepository {
	t.Helper()
	db := sql.OpenDB(contractConnector{state})
	t.Cleanup(func() { db.Close() })
	return NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestExistingOptionContractDoesNotFetch(t *testing.T) {
	t.Setenv("DATA_FETCHER_URL", "")
	state := &contractDB{existing: true, strike: 2500000}
	r := testRepository(t, state)
	contract, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
	if err != nil || contract.ID != 101 || contract.ExpiryID != 9 || contract.UnderlyingID != 7 || contract.StrikePrice != state.strike || !contract.Expiry.Equal(testExpiry) || contract.OptionType != models.Call {
		t.Fatalf("contract=%+v err=%v", contract, err)
	}
	if state.queries != 1 || state.symbols != 0 {
		t.Fatalf("queries=%d symbol lookups=%d", state.queries, state.symbols)
	}
}

func TestMissingOptionContractFetchesAndRequeries(t *testing.T) {
	for _, tt := range []struct {
		strike         models.Price
		rupees, status string
	}{
		{2500000, "25000.00", "ingested"},
		{2500005, "25000.05", "already_exists"},
		{29, "0.29", "ingested"},
		{models.Price(math.MaxInt64), "92233720368547758.07", "ingested"},
	} {
		t.Run(tt.rupees, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				if req.Method != http.MethodPost || req.URL.Path != "/options/fetch" || req.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected HTTP request: %s %s", req.Method, req.URL.Path)
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body) != 4 || string(body["underlying"]) != `"NIFTY"` || string(body["expiry"]) != `"2025-12-30"` || string(body["option_type"]) != `"CALL"` || string(body["strike"]) != tt.rupees {
					t.Errorf("incorrect request: %s", body)
				}
				fmt.Fprintf(w, `{"status":%q,"contract_id":101,"fetched":100,"inserted":100}`, tt.status)
			}))
			defer server.Close()
			t.Setenv("DATA_FETCHER_URL", server.URL+"/")
			state := &contractDB{strike: tt.strike, inserted: true}
			r := testRepository(t, state)
			contract, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
			if err != nil || contract.ID != 101 || contract.StrikePrice != tt.strike {
				t.Fatalf("contract=%+v err=%v", contract, err)
			}
			if calls.Load() != 1 || state.queries != 2 || state.symbols != 1 {
				t.Fatalf("calls=%d queries=%d symbols=%d", calls.Load(), state.queries, state.symbols)
			}
		})
	}
}

func TestDatabaseErrorDoesNotFetch(t *testing.T) {
	failure := errors.New("database unavailable")
	state := &contractDB{strike: 2500000, initialErr: failure}
	r := testRepository(t, state)
	_, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
	if !errors.Is(err, failure) || state.queries != 1 || state.symbols != 0 {
		t.Fatalf("err=%v queries=%d symbols=%d", err, state.queries, state.symbols)
	}
}

func TestFetcherFailureDoesNotRequery(t *testing.T) {
	for _, tt := range []struct {
		name       string
		code       int
		body, want string
	}{
		{"python error", 400, `{"error":"Expiry must already exist"}`, "Expiry must already exist"},
		{"upstream error", 502, `{"error":"Breeze unavailable"}`, "502 Bad Gateway"},
		{"invalid error body", 500, "unavailable", "500 Internal Server Error"},
		{"invalid success body", 200, "not JSON", "decode option ingestion response"},
		{"not completed", 202, `{"status":"queued","contract_id":101}`, "did not confirm completed ingestion"},
		{"missing ID", 200, `{"status":"ingested"}`, "did not confirm completed ingestion"},
		{"redirect", 307, "", "307 Temporary Redirect"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(tt.code)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()
			t.Setenv("DATA_FETCHER_URL", server.URL)
			state := &contractDB{strike: 2500000}
			r := testRepository(t, state)
			_, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
			if err == nil || !strings.Contains(err.Error(), tt.want) || state.queries != 1 || calls.Load() != 1 {
				t.Fatalf("err=%v queries=%d calls=%d", err, state.queries, calls.Load())
			}
		})
	}
}

func TestContractStillMissingAfterIngestion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"status":"ingested","contract_id":101}`)
	}))
	defer server.Close()
	t.Setenv("DATA_FETCHER_URL", server.URL)
	state := &contractDB{strike: 2500000}
	r := testRepository(t, state)
	_, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
	if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), "still missing after successful ingestion") || state.queries != 2 {
		t.Fatalf("err=%v queries=%d", err, state.queries)
	}
}

func TestMissingFetcherConfiguration(t *testing.T) {
	t.Setenv("DATA_FETCHER_URL", "")
	state := &contractDB{strike: 2500000}
	r := testRepository(t, state)
	_, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
	if err == nil || !strings.Contains(err.Error(), "DATA_FETCHER_URL is required") || state.queries != 1 {
		t.Fatalf("err=%v queries=%d", err, state.queries)
	}
}

func TestUnderlyingLookupFailureDoesNotFetch(t *testing.T) {
	failure := errors.New("underlying lookup failed")
	state := &contractDB{strike: 2500000, symbolErr: failure}
	r := testRepository(t, state)
	_, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "resolve underlying symbol") || state.queries != 1 || state.symbols != 1 {
		t.Fatalf("err=%v queries=%d symbols=%d", err, state.queries, state.symbols)
	}
}

func TestDatabaseErrorAfterIngestion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, `{"status":"ingested","contract_id":101}`)
	}))
	defer server.Close()
	t.Setenv("DATA_FETCHER_URL", server.URL)
	failure := errors.New("database unavailable after ingestion")
	state := &contractDB{strike: 2500000, afterErr: failure}
	r := testRepository(t, state)
	_, err := r.GetOptionContract(context.Background(), 7, testExpiry, state.strike, models.Call)
	if !errors.Is(err, failure) || state.queries != 2 {
		t.Fatalf("err=%v queries=%d", err, state.queries)
	}
}

func TestIngestionHTTPTimeoutAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint("cancelled=", cancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if cancelled {
					cancel()
				}
				<-release
			}))
			defer server.Close()
			defer close(release)
			t.Setenv("DATA_FETCHER_URL", server.URL)
			state := &contractDB{strike: 2500000}
			r := testRepository(t, state)
			r.httpClient.Timeout = 100 * time.Millisecond
			_, err := r.GetOptionContract(ctx, 7, testExpiry, state.strike, models.Call)
			want := context.DeadlineExceeded
			if cancelled {
				want = context.Canceled
			}
			if !errors.Is(err, want) || state.queries != 1 {
				t.Fatalf("err=%v; want %v; queries=%d", err, want, state.queries)
			}
		})
	}
}

type contractConnector struct{ state *contractDB }
type contractConn struct{ state *contractDB }
type contractDriver struct{}

func (c contractConnector) Connect(context.Context) (driver.Conn, error) {
	return contractConn{c.state}, nil
}
func (contractConnector) Driver() driver.Driver          { return contractDriver{} }
func (contractDriver) Open(string) (driver.Conn, error)  { return nil, errors.New("use connector") }
func (contractConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (contractConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }
func (contractConn) Close() error                        { return nil }
func (c contractConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.state.query(ctx, query, args)
}

type contractRows struct {
	columns []string
	row     []driver.Value
}

func (r *contractRows) Columns() []string { return r.columns }
func (*contractRows) Close() error        { return nil }
func (r *contractRows) Next(dest []driver.Value) error {
	if r.row == nil {
		return io.EOF
	}
	copy(dest, r.row)
	r.row = nil
	return nil
}
