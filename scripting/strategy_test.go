package scripting

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/models"
	"optionsbacktest/repository"
)

const setup = `
def initialize(ctx):
    ctx.configure(underlying="NIFTY", start_date="2026-09-01", end_date="2026-10-31")
    ctx.register_leg("CE", quantity=75)
`

const trading = `
def on_candle(ctx):
    leg = ctx.leg("CE")
    if ctx.dte == 45:
        if ctx.underlying_candle.start_time.endswith("09:15:00+05:30"):
            if leg.status != "PENDING" or leg.selected_contract != None or leg.entry_price != None:
                fail("state leaked across expiry cycles")
            ctx.close("CE")  # Closing a pending leg is a no-op.
        ctx.sell_call("CE", offset=2)
        ctx.sell_call("CE", offset=2)
    if ctx.dte == 21:
        ctx.close("CE")
        ctx.close("CE")
    if leg.status == "OPEN":
        if leg.entry_price != 10000 or leg.quantity != 75 or leg.option_candle == None:
            fail("incorrect open leg snapshot")
        if leg.selected_contract.strike_price != 2265000:
            fail("incorrect selected strike")
    if leg.status == "CLOSED":
        ctx.close("CE")
        ctx.sell_call("CE", offset=2)
    if ctx.legs["CE"].status != ctx.leg("CE").status:
        fail("inconsistent leg views")
`

func TestExampleScriptLoads(t *testing.T) {
	script, err := Load(context.Background(), "../strategies/short_call.star")
	if err != nil {
		t.Fatal(err)
	}
	if script.context.underlying != "NIFTY" || script.context.legs[0].Quantity != 75 {
		t.Fatal("example script configuration was not loaded")
	}
}

func TestInitialization(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"valid", setup + "\ndef on_candle(ctx):\n    pass\n", ""},
		{"missing initialize", "def on_candle(ctx):\n    pass\n", "initialize(ctx)"},
		{"missing callback", setup, "on_candle(ctx)"},
		{"missing configuration", "def initialize(ctx):\n    ctx.register_leg('CE', 1)\ndef on_candle(ctx):\n    pass\n", "configure"},
		{"duplicate leg", setup + "    ctx.register_leg('CE', 2)\ndef on_candle(ctx):\n    pass\n", "already registered"},
		{"invalid quantity", strings.Replace(setup, "quantity=75", "quantity=0", 1) + "\ndef on_candle(ctx):\n    pass\n", "positive"},
		{"invalid dates", strings.Replace(setup, "2026-09-01", "2027-09-01", 1) + "\ndef on_candle(ctx):\n    pass\n", "start_date"},
		{"entry during initialization", setup + "    ctx.sell_call('CE', offset=2)\ndef on_candle(ctx):\n    pass\n", "only available during on_candle"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			script, err := load(context.Background(), "test.star", tt.source)
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("error=%v; want %s", err, tt.want)
				}
				return
			}
			if err != nil || !script.context.configured || script.context.initializing || len(script.context.legs) != 1 {
				t.Fatalf("initialization failed: %v", err)
			}
		})
	}
}

func TestEntryExitAndExpiryIsolation(t *testing.T) {
	f := newFixture(t)
	script, err := load(context.Background(), "test.star", setup+trading)
	if err != nil {
		t.Fatal(err)
	}
	results, err := script.Run(context.Background(), engine.NewEngine(f.repo))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || f.contractQueries != 2 {
		t.Fatalf("cycles=%d contract queries=%d", len(results), f.contractQueries)
	}
	for i, cycle := range results {
		if len(cycle) != 1 || cycle[0].Status != models.LegClosed || len(cycle[0].Trades) != 1 {
			t.Fatalf("unexpected lifecycle: %+v", cycle)
		}
		trade := cycle[0].Trades[0]
		entry := f.expiries[i].AddDate(0, 0, -45).Add(9*time.Hour + 20*time.Minute)
		exit := f.expiries[i].AddDate(0, 0, -21).Add(9*time.Hour + 20*time.Minute)
		if trade.ContractID != int64(100+i) || trade.Side != models.Short || trade.Quantity != 75 || trade.Status != models.TradeClosed || !trade.EntryTime.Equal(entry) || !trade.ExitTime.Equal(exit) || trade.EntryPrice != 10000 || trade.ExitPrice != 6000 || trade.PnL != 300000 {
			t.Fatalf("incorrect trade: %+v", trade)
		}
	}
	if results[0][0].Leg == results[1][0].Leg {
		t.Fatal("expiry cycles share leg configuration pointers")
	}
}

func TestMissingUnderlying(t *testing.T) {
	f := newFixture(t)
	script, err := load(context.Background(), "test.star", strings.Replace(setup, "NIFTY", "UNKNOWN", 1)+trading)
	if err != nil {
		t.Fatal(err)
	}
	_, err = script.Run(context.Background(), engine.NewEngine(f.repo))
	if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), "UNKNOWN") {
		t.Fatalf("expected wrapped missing-underlying error, got %v", err)
	}
}

func TestRuntimeErrors(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"late registration", "ctx.register_leg('late', 1)", "only allowed during initialize"},
		{"late configuration", "ctx.configure(underlying='NIFTY', start_date='2026-01-01', end_date='2026-12-31')", "allowed once during initialize"},
		{"unknown entry", "ctx.sell_call('unknown', offset=2)", "unknown leg"},
		{"unknown close", "ctx.close('unknown')", "unknown leg"},
		{"unknown lookup", "ctx.leg('unknown')", "unknown leg"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			script, err := load(context.Background(), "test.star", setup+"\ndef on_candle(ctx):\n    "+tt.body+"\n")
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

func TestMissingOptionData(t *testing.T) {
	for _, dte := range []int{45, 21} {
		f := newFixture(t)
		f.missingDTE = dte
		script, err := load(context.Background(), "test.star", setup+trading)
		if err != nil {
			t.Fatal(err)
		}
		result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
		if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), "missing option candle") {
			t.Fatalf("missing data was not explicit: %v", err)
		}
		if dte == 45 && len(result[0][0].Trades) != 0 {
			t.Fatal("missing entry candle created a trade")
		}
		if dte == 21 && result[0][0].Trades[0].Status != models.TradeOpen {
			t.Fatal("missing exit data closed a trade")
		}
	}
}

type fixture struct {
	t               *testing.T
	repo            *repository.MarketDataRepository
	expiries        []time.Time
	contractQueries int
	missingDTE      int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, expiries: []time.Time{time.Date(2026, 9, 29, 0, 0, 0, 0, location), time.Date(2026, 10, 27, 0, 0, 0, 0, location)}}
	db := sql.OpenDB(connector{query: f.query})
	t.Cleanup(func() { db.Close() })
	f.repo = repository.NewMarketDataRepository(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return f
}

func (f *fixture) query(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	r := &rows{}
	switch {
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
		f.contractQueries++
		r.columns = []string{"id", "underlying_id", "expiry_id", "expiry_date", "strike_price", "option_type"}
		for i, expiry := range f.expiries {
			if expiry.Equal(args[1].Value.(time.Time)) {
				if args[2].Value != int64(2265000) || args[3].Value != "CALL" {
					f.t.Fatalf("offset selection not used: %+v", args)
				}
				r.values = [][]driver.Value{{int64(100 + i), int64(7), int64(i + 1), expiry, int64(2265000), "CALL"}}
			}
		}
	case strings.Contains(query, "FROM lab.underlying_candles"):
		r.columns = candleColumns()
		start := args[2].Value.(time.Time)
		for _, expiry := range f.expiries {
			for _, dte := range []int{45, 21} {
				if start.Equal(expiry.AddDate(0, 0, -dte)) {
					for _, minute := range []int{15, 20, 25} {
						r.values = append(r.values, candleRow(start.Add(9*time.Hour+time.Duration(minute)*time.Minute), 2253000))
					}
				}
			}
		}
	case strings.Contains(query, "FROM lab.option_candles"):
		r.columns = candleColumns()
		at := args[2].Value.(time.Time)
		if !at.Equal(args[3].Value.(time.Time)) || args[1].Value != "5m" {
			f.t.Fatal("option query could expose future candles")
		}
		expiry := f.expiries[args[0].Value.(int64)-100]
		day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
		dte := int(expiry.Sub(day) / (24 * time.Hour))
		if dte != f.missingDTE {
			price := int64(10000)
			if dte == 21 {
				price = 6000
			}
			r.values = [][]driver.Value{candleRow(at, price)}
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
