package scripting

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"optionsbacktest/engine"
	"optionsbacktest/models"
	"optionsbacktest/repository"
)

const setup = `
def initialize(ctx):
    ctx.configure(underlying="NIFTY", start_date="2026-09-01", end_date="2026-10-31")
`

const trading = `
def on_candle(ctx):
    if ctx.dte == 45 and "entry" not in ctx.state:
        ctx.state["entry"] = ctx.place_order(contract=atm_option("CALL", offset=2), side="SELL", quantity=75)
        order = ctx.order(ctx.state["entry"])
        if order.status != "PENDING" or order.contract_id != None:
            fail("placement selected or executed a contract")
    if ctx.dte == 21 and "exit" not in ctx.state:
        entry = ctx.order(ctx.state["entry"])
        if entry.status == "FILLED":
            ctx.state["exit"] = ctx.place_order(contract=entry.contract_id, side="BUY", quantity=75)
`

func TestExampleScriptLoads(t *testing.T) {
	script, err := Load("../strategies/short_call.star")
	if err != nil {
		t.Fatal(err)
	}
	if script.context.underlying != "NIFTY" || !script.context.configured {
		t.Fatal("configuration not loaded")
	}
}

func TestInitialization(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"valid", setup + "\ndef on_candle(ctx):\n    pass\n", ""},
		{"missing initialize", "def on_candle(ctx):\n    pass\n", "initialize(ctx)"},
		{"missing callback", setup, "on_candle(ctx)"},
		{"missing configuration", "def initialize(ctx):\n    pass\ndef on_candle(ctx):\n    pass\n", "configure"},
		{"invalid dates", strings.Replace(setup, "2026-09-01", "2027-09-01", 1) + "\ndef on_candle(ctx):\n    pass\n", "start_date"},
		{"entry during initialization", setup + "    ctx.place_order(contract=atm_option('CALL'),side='SELL',quantity=75)\ndef on_candle(ctx):\n    pass\n", "only available during on_candle"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			script, err := load("test.star", tt.source)
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

func TestEntryExitAndExpiryIsolation(t *testing.T) {
	f := newFixture(t)
	script, err := load("test.star", setup+trading)
	if err != nil {
		t.Fatal(err)
	}
	f.onContractQuery = func() {
		if script.context.execution.Time.Format("15:04") != "09:20" {
			t.Fatal("selection did not wait for next advance")
		}
	}
	result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
	if err != nil {
		t.Fatal(err)
	}
	if result.Underlying != "NIFTY" || len(result.Executions) != 2 || f.contractQueries != 2 {
		t.Fatal("incorrect expiry count")
	}
	for i, cycle := range result.Executions {
		if len(cycle) != 1 {
			t.Fatal("state leaked between expiries or order duplicated")
		}
		position := cycle[0]
		entry := f.expiries[i].AddDate(0, 0, -45).Add(9*time.Hour + 15*time.Minute)
		exit := f.expiries[i].AddDate(0, 0, -21).Add(9*time.Hour + 15*time.Minute)
		if position.Contract.ID != int64(100+i) || position.Contract.StrikePrice != 2265000 || position.Side != models.Short || position.Quantity != 75 || position.Status != models.TradeClosed || !position.EntryTime.Equal(entry) || !position.ExitTime.Equal(exit) || position.EntryPrice != 10000 || position.ExitPrice != 6000 || position.PnL != 300000 {
			t.Fatalf("incorrect position: %+v", position)
		}
	}
	if result.Executions[0][0] == result.Executions[1][0] {
		t.Fatal("shared position state")
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

func TestSessionCloseAdvancesWithoutStrategyCallback(t *testing.T) {
	f := newFixture(t)
	source := setup + `
def on_candle(ctx):
    if ctx.time >= "15:30":
        fail("callback at session close")
    if ctx.dte == 45 and ctx.time == "15:25":
        ctx.state["entry"] = ctx.place_order(contract=atm_option("CALL", offset=2), side="SELL", quantity=75)
    if ctx.dte == 21 and ctx.time == "15:25":
        ctx.place_order(contract=ctx.order(ctx.state["entry"]).contract_id, side="BUY", quantity=75)
`
	script, err := load("test.star", source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
	if err != nil {
		t.Fatal(err)
	}
	for _, cycle := range result.Executions {
		position := cycle[0]
		if position.Status != models.TradeClosed || position.EntryTime.Format("15:04") != "15:25" || position.ExitTime.Format("15:04") != "15:25" {
			t.Fatalf("incorrect final advance: %+v", position)
		}
	}
}

func TestMissingUnderlying(t *testing.T) {
	f := newFixture(t)
	script, err := load("test.star", strings.Replace(setup, "NIFTY", "UNKNOWN", 1)+trading)
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
		{"zero quantity", "ctx.place_order(contract=atm_option('CALL'),side='SELL',quantity=0)", "positive"},
		{"negative quantity", "ctx.place_order(contract=atm_option('CALL'),side='SELL',quantity=-1)", "positive"},
		{"missing quantity", "ctx.place_order(contract=atm_option('CALL'),side='SELL')", "quantity"},
		{"bad quantity type", "ctx.place_order(contract=1,side='SELL',quantity=1.5)", "quantity"},
		{"bad option type", "ctx.place_order(contract=atm_option('CE'),side='SELL',quantity=75)", "option_type"},
		{"bad side", "ctx.place_order(contract=atm_option('CALL'),side='SHORT',quantity=75)", "side"},
		{"bad offset", "ctx.place_order(contract=atm_option('CALL',offset='2'),side='SELL',quantity=75)", "offset"},
		{"missing contract", "ctx.place_order(side='SELL',quantity=75)", "contract"},
		{"bad contract type", "ctx.place_order(contract='101',side='SELL',quantity=75)", "contract"},
		{"negative contract ID", "ctx.place_order(contract=-1,side='SELL',quantity=75)", "positive"},
		{"zero contract ID", "ctx.place_order(contract=0,side='SELL',quantity=75)", "positive"},
		{"large contract ID", "ctx.place_order(contract=9223372036854775808,side='SELL',quantity=75)", "int64"},
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

func TestMissingOptionCandleHandling(t *testing.T) {
	for _, dte := range []int{45, 21} {
		f := newFixture(t)
		f.missingDTE, f.missingMinute = dte, 15
		script, err := load("test.star", setup+trading)
		if err != nil {
			t.Fatal(err)
		}
		result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
		if dte == 21 {
			if !errors.Is(err, sql.ErrNoRows) || !strings.Contains(err.Error(), "missing completed option candle") {
				t.Fatalf("missing candle error: %v", err)
			}
			if result.Executions[0][0].Candle != nil || result.Executions[0][0].Status != models.TradeOpen {
				t.Fatal("stale snapshot or premature exit")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, cycle := range result.Executions {
			if cycle[0].EntryTime.Minute() != 20 || cycle[0].ExitTime.Minute() != 15 || cycle[0].Status != models.TradeClosed {
				t.Fatalf("incorrect pending fill: %+v", cycle[0])
			}
		}
	}
}

func TestTypesSidesAndDefaultOffset(t *testing.T) {
	for _, optionType := range []models.OptionType{models.Call, models.Put} {
		for _, side := range []models.PositionSide{models.Long, models.Short} {
			t.Run(string(optionType)+"/"+string(side), func(t *testing.T) {
				f := newFixture(t)
				f.expectedStrike, f.expectedType = 2255000, string(optionType)
				entrySide, exitSide := "SELL", "BUY"
				if side == models.Long {
					entrySide, exitSide = "BUY", "SELL"
				}
				source := setup + fmt.Sprintf(`
def on_candle(ctx):
    if ctx.dte == 45 and "entry" not in ctx.state:
        ctx.state["entry"] = ctx.place_order(contract=atm_option(%q),side=%q,quantity=75)
    if ctx.dte == 21 and "exit" not in ctx.state:
        ctx.state["exit"] = ctx.place_order(contract=ctx.order(ctx.state["entry"]).contract_id,side=%q,quantity=75)
`, string(optionType), entrySide, exitSide)
				script, err := load("test.star", source)
				if err != nil {
					t.Fatal(err)
				}
				result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
				if err != nil {
					t.Fatal(err)
				}
				for _, cycle := range result.Executions {
					position := cycle[0]
					want := models.Price(300000)
					if side == models.Long {
						want = -want
					}
					if position.Contract.OptionType != optionType || position.Side != side || position.Status != models.TradeClosed || position.PnL != want {
						t.Fatalf("incorrect position: %+v", position)
					}
				}
			})
		}
	}
}

func TestPlaceOrderReturnsUniqueIDsWithoutImmediateExecution(t *testing.T) {
	f := newFixture(t)
	script, err := load("test.star", setup+`
def on_candle(ctx):
    first = ctx.place_order(contract=atm_option("CALL",offset=2),side="SELL",quantity=75)
    second = ctx.place_order(contract=atm_option("CALL",offset=2),side="SELL",quantity=75)
    if type(first) != "string" or not first or first == second:
        fail("invalid order IDs")
    if ctx.order(first).status != "PENDING" or ctx.order(first).contract_id != None:
        fail("order was executed or selected during placement")
`)
	if err != nil {
		t.Fatal(err)
	}
	stop := errors.New("placement verified")
	_, err = engine.NewEngine(f.repo).Backtest(context.Background(), "NIFTY", script.context.start, script.context.end, func(execution *engine.Execution) error {
		script.context.execution = execution
		_, err := starlark.Call(script.thread, script.onCandle, starlark.Tuple{script.context}, nil)
		if err != nil {
			return err
		}
		if f.contractQueries != 0 || len(execution.Orders) != 2 || len(execution.Positions) != 0 {
			t.Fatal("placement performed selection or execution")
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("unexpected result: %v", err)
	}
}

type fixture struct {
	t               *testing.T
	repo            *repository.MarketDataRepository
	expiries        []time.Time
	contractQueries int
	onContractQuery func()
	missingDTE      int
	missingMinute   int
	expectedStrike  int64
	expectedType    string
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

func TestExampleStrategiesBacktest(t *testing.T) {
	for _, tt := range []struct {
		file      string
		positions int
	}{{"short_call.star", 1}, {"short_strangle.star", 2}} {
		t.Run(tt.file, func(t *testing.T) {
			source, err := os.ReadFile("../strategies/" + tt.file)
			if err != nil {
				t.Fatal(err)
			}
			// Retain the actual example's logic while selecting the fixture's expiry range.
			text := strings.ReplaceAll(string(source), "2026-01-01", "2026-09-01")
			text = strings.ReplaceAll(text, "2026-06-30", "2026-10-31")
			script, err := load("example.star", text)
			if err != nil {
				t.Fatal(err)
			}
			f := newFixture(t)
			result, err := script.Run(context.Background(), engine.NewEngine(f.repo))
			if err != nil {
				t.Fatal(err)
			}
			for _, cycle := range result.Executions {
				if len(cycle) != tt.positions {
					t.Fatalf("expected %d positions per expiry, got %d", tt.positions, len(cycle))
				}
				for i, position := range cycle {
					optionType, strike := models.Call, models.Price(2265000)
					if i == 1 {
						optionType, strike = models.Put, 2245000
					}
					if position.Contract.OptionType != optionType || position.Contract.StrikePrice != strike || position.Side != models.Short || position.Quantity != 75 || position.EntryTime.Format("15:04") != "09:30" || position.ExitTime.Format("15:04") != "15:15" || position.PnL != 300000 || position.Status != models.TradeClosed {
						t.Fatalf("incorrect example lifecycle: %+v", position)
					}
				}
				if len(cycle) == 2 && cycle[0].ContractID == cycle[1].ContractID {
					t.Fatal("call and put must use distinct contracts")
				}
			}

		})
	}
}
