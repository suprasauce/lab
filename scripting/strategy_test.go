package scripting

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/repository"
)

const setup = `
def initialize(ctx):
    ctx.configure(underlying="NIFTY")
    for expiry in ctx.get_expiries(underlying="NIFTY", start="2026-09-01", end="2026-10-31"):
        ctx.simulate(expiry)
`

func TestLoadStrategyFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "strategy.star")
	if err := os.WriteFile(filename, []byte(setup+"\ndef on_candle(ctx):\n    pass\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if script.context != nil {
		t.Fatal("initialization ran during load")
	}
}

func TestInitialization(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"valid", setup + "\ndef on_candle(ctx):\n    pass\n", ""},
		{"missing initialize", "def on_candle(ctx):\n    pass\n", "initialize(ctx)"},
		{"missing callback", setup, "on_candle(ctx)"},
		{"missing configuration", "def initialize(ctx):\n    pass\ndef on_candle(ctx):\n    pass\n", "configure"},
		{"invalid dates", strings.Replace(setup, "2026-09-01", "2027-09-01", 1) + "\ndef on_candle(ctx):\n    pass\n", "start_date"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			script, err := load("test.star", tt.source)
			if err == nil {
				f := newFixture(t)
				_, err = script.Run(context.Background(), engine.NewEngine(f.repo))
			}
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("error=%v; want %s", err, tt.want)
				}
				return
			}
			if err != nil || !script.context.configured || script.context.initializing {
				t.Fatalf("initialization failed: %v", err)
			}
		})
	}
}

func TestNoEntryCreatesNoPositions(t *testing.T) {
	f := newFixture(t)
	script, err := load("test.star", setup+"\ndef on_candle(ctx):\n    pass\n")
	if err != nil {
		t.Fatal(err)
	}
	result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Executions) != 2 || len(result.Executions[0]) != 0 || len(result.Executions[1]) != 0 || f.contractQueries != 0 {
		t.Fatal("created positions without orders")
	}
}

func TestStatePersistsAndStartsEmptyEveryExpiry(t *testing.T) {
	source := setup + `    ctx.state["initial"] = 123

def on_candle(ctx):
    if ctx.dte == 45 and ctx.time == "09:15":
        if len(ctx.state) != 0:
            fail("expiry state must start empty")
        ctx.state["counter"] = 0
        ctx.state["label"] = "fresh"
        ctx.state[1] = {"items": [], "pair": (1, "value")}
    else:
        if "counter" not in ctx.state or ctx.state["label"] != "fresh":
            fail("state did not persist across callbacks")
    ctx.state[1]["items"].append(ctx.state["counter"])
    ctx.state["counter"] += 1
    if len(ctx.state[1]["items"]) != ctx.state["counter"]:
        fail("nested state did not persist")
`
	script, err := load("test.star", source)
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		f := newFixture(t)
		if _, err := script.Run(context.Background(), engine.NewEngine(f.repo)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMissingUnderlying(t *testing.T) {
	f := newFixture(t)
	script, err := load("test.star", strings.Replace(setup, "NIFTY", "UNKNOWN", 1)+"\ndef on_candle(ctx):\n    pass\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = script.Run(context.Background(), engine.NewEngine(f.repo))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing underlying error: %v", err)
	}
}

func TestRuntimeErrors(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"late configuration", "ctx.configure(underlying='NIFTY',start_date='2026-01-01',end_date='2026-12-31')", "allowed once during initialize"},
		{"bad quantity type", "ctx.place_order(tradingsymbol='NIFTY26NOV26000CE',side='SELL',quantity=1.5)", "quantity"},
		{"missing trading symbol", "ctx.place_order(side='SELL',quantity=75)", "tradingsymbol"},
		{"unknown order", "ctx.order('unknown')", "unknown order"},
		{"removed entry API", "ctx.open('L')", "no .open"},
		{"removed exit API", "ctx.close('L')", "no .close"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			script, err := load("test.star", setup+"\ndef on_candle(ctx):\n    "+tt.body+"\n")
			if err != nil {
				t.Fatal(err)
			}
			_, err = script.Run(context.Background(), engine.NewEngine(f.repo))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v; want %s", err, tt.want)
			}
		})
	}
}

func TestRemovedATMOptionBuiltinIsUnavailable(t *testing.T) {
	_, err := load("test.star", setup+"\ndef on_candle(ctx):\n    atm_option('CALL')\n")
	if err == nil || !strings.Contains(err.Error(), "atm_option") {
		t.Fatalf("removed builtin is still available: %v", err)
	}
}

type fixture struct {
	t                 *testing.T
	repo              *repository.Repository
	expiries          []time.Time
	contractQueries   int
	onContractQuery   func()
	missingDTE        int
	missingMinute     int
	missingAllOptions bool
	missingExits      bool
	expectedStrike    int64
	expectedType      string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, expiries: []time.Time{time.Date(2026, 9, 29, 0, 0, 0, 0, location), time.Date(2026, 10, 27, 0, 0, 0, 0, location)}}
	f.expectedStrike, f.expectedType = 2265000, "CALL"
	db := sql.OpenDB(connector{query: f.query})
	t.Cleanup(func() { db.Close() })
	f.repo = repository.NewRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return f
}

func (f *fixture) query(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	r := &rows{}
	switch {
	case strings.Contains(query, "SELECT c.tradingsymbol"):
		f.contractQueries++
		if args[0].Value != int64(7) || args[2].Value != int64(2265000) || args[3].Value != "PUT" {
			f.t.Fatalf("unexpected symbol lookup: %v", args)
		}
		r.columns = []string{"tradingsymbol"}
		r.values = [][]driver.Value{{"NIFTY-20260127-22650-PE"}}
	case strings.Contains(query, "e.option_chain_fetched"):
		r.columns = []string{"id", "option_chain_fetched"}
		r.values = [][]driver.Value{{int64(9), true}}
	case strings.Contains(query, "FROM lab.underlying_candles") && strings.Contains(query, "ORDER BY start_time DESC"):
		r.columns = []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}
		at := args[1].Value.(time.Time)
		r.values = [][]driver.Value{candleRow(at.Add(-5*time.Minute), 2253000)}
	case strings.Contains(query, "FROM lab.underlyings"):
		if !strings.Contains(query, "WHERE symbol = $1") || len(args) != 1 {
			f.t.Fatal("underlying lookup must filter by symbol")
		}
		r.columns = []string{"id", "symbol"}
		if args[0].Value == "NIFTY" {
			r.values = [][]driver.Value{{int64(7), "NIFTY"}}
		}
	case strings.Contains(query, "FROM lab.option_expiry"):
		if args[0].Value != int64(7) || args[1].Value.(time.Time).Format(time.DateOnly) != "2026-09-01" || args[2].Value.(time.Time).Format(time.DateOnly) != "2026-10-31" {
			f.t.Fatalf("script configuration not used: %+v", args)
		}
		r.columns = []string{"expiry_date"}
		for _, expiry := range f.expiries {
			r.values = append(r.values, []driver.Value{expiry})
		}
	case strings.Contains(query, "FROM lab.option_contracts"):
		if len(args) == 1 {
			r.columns = []string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}
			for i, expiry := range f.expiries {
				if args[0].Value == int64(100+i) {
					r.values = [][]driver.Value{{int64(100 + i), int64(7), int64(i + 1), expiry, f.expectedStrike, f.expectedType}}
				}
			}
			for i, expiry := range f.expiries {
				if args[0].Value == int64(200+i) {
					r.values = [][]driver.Value{{int64(200 + i), int64(7), int64(i + 1), expiry, int64(2245000), "PUT"}}
				}
			}
			return r, nil
		}
		f.contractQueries++
		if f.onContractQuery != nil {
			f.onContractQuery()
		}
		r.columns = []string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}
		for i, expiry := range f.expiries {
			if expiry.Equal(args[1].Value.(time.Time)) {
				strike, optionType, id := f.expectedStrike, f.expectedType, int64(100+i)
				if args[3].Value == "PUT" && f.expectedType == "CALL" {
					strike, optionType, id = 2245000, "PUT", int64(200+i)
				}
				if args[2].Value != strike || args[3].Value != optionType {
					f.t.Fatalf("offset selection not used: %+v", args)
				}
				r.values = [][]driver.Value{{id, int64(7), int64(i + 1), expiry, strike, optionType}}
			}
		}
	case strings.Contains(query, "FROM lab.underlying_candles"):
		r.columns = candleColumns()
		start := args[2].Value.(time.Time)
		var first []driver.Value
		for _, expiry := range f.expiries {
			for _, dte := range []int{45, 21} {
				for slot := 0; slot < 75; slot++ {
					at := expiry.AddDate(0, 0, -dte).Add(9*time.Hour + 15*time.Minute + time.Duration(slot)*5*time.Minute)
					if at.Before(start) {
						continue
					}
					if len(args) == 3 {
						if first == nil || at.Before(first[0].(time.Time)) {
							first = candleRow(at, 2253000)
						}
					} else if at.Before(args[3].Value.(time.Time)) {
						r.values = append(r.values, candleRow(at, 2253000))
					}
				}
			}
		}
		if len(args) == 3 && first != nil {
			r.values = [][]driver.Value{first}
		}
	case strings.Contains(query, "FROM lab.option_candles"):
		r.columns = candleColumns()
		if f.missingAllOptions {
			return r, nil
		}
		at := args[2].Value.(time.Time)
		if args[1].Value != "5m" {
			f.t.Fatal("unexpected timeframe")
		}
		end := args[3].Value.(time.Time)
		rounded := at.Truncate(5 * time.Minute)
		if rounded.Before(at) {
			rounded = rounded.Add(5 * time.Minute)
		}
		for at = rounded; !at.After(end); at = at.Add(5 * time.Minute) {
			expiry := f.expiries[args[0].Value.(int64)%100]
			day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
			dte := int(expiry.Sub(day) / (24 * time.Hour))
			if f.missingExits && dte <= 21 {
				continue
			}
			if dte != f.missingDTE || at.Hour() != 9 || at.Minute() != f.missingMinute {
				price := int64(10000)
				if dte == 21 {
					price = 6000
				}
				r.values = [][]driver.Value{candleRow(at, price)}
				break
			}
		}
	default:
		f.t.Fatalf("unexpected query: %s", query)
	}
	return r, nil
}

func candleColumns() []string {
	return []string{"start_time", "timeframe", "open", "high", "low", "close", "volume"}
}
func candleRow(t time.Time, price int64) []driver.Value {
	return []driver.Value{t, "5m", price, price + 100, price - 100, price + 50, int64(0)}
}

type connector struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}
type connection struct{ connector }
type sqlDriver struct{}

func (c connector) Connect(context.Context) (driver.Conn, error) { return connection{c}, nil }
func (connector) Driver() driver.Driver                          { return sqlDriver{} }
func (sqlDriver) Open(string) (driver.Conn, error)               { return nil, errors.New("use connector") }
func (connection) Prepare(string) (driver.Stmt, error)           { return nil, errors.New("not implemented") }
func (connection) Begin() (driver.Tx, error)                     { return nil, errors.New("not implemented") }
func (connection) Close() error                                  { return nil }
func (c connection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type rows struct {
	columns []string
	values  [][]driver.Value
}

func (r *rows) Columns() []string { return r.columns }
func (*rows) Close() error        { return nil }
func (r *rows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}
