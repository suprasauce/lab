# Options backtesting

Go execution engine with Starlark strategy definitions and PostgreSQL market data.

- `models`: underlying, option, expiry, strategy, leg, trade, and candle.
- `engine`: expiry simulation clock, short-call execution, position state, and trade history.
- `scripting`: Starlark loading, initialization, context bindings, and candle callbacks.
- `strategies`: strategy scripts, including the 45-DTE entry / 21-DTE exit example.
- `selector`: strike selection, with offset selection implemented and premium/delta placeholders.
- `database`: reads DATABASE_URL and initializes and verifies the PostgreSQL pool using pgx.
- `repository`: market data queries using an injected database/sql pool.
- `logging`: configurable slog JSON logger writing to stdout; INFO by default in main.
- `main.go`: initializes dependencies, loads the selected script, runs it, and closes the pool.

Underlying-to-instrument relationships use `UnderlyingID`. The optional Strategy
metadata model contains only ID and name. The script supplies all run configuration.
OptionLeg stores a logical leg's name and quantity supplied on its first entry request; LegExecution stores
independent status and trade history for each expiry. Each trade stores a position's
contract, entry and exit details, side, quantity, status, and P&L field.
Underlying IDs and option contract IDs use int64 to match PostgreSQL BIGINT;
other IDs currently remain strings. No candle ownership mapping is defined yet.

Prices use int64 amounts in paise (100 paise = 1 rupee). Convert rupees to
paise before assigning prices. Quantities count units rather than lots.

Option and Underlying are concrete market-data models. Leg configuration does
not reference a generic instrument abstraction.

OptionExpiry stores each underlying's expiry date and explicit MONTHLY/WEEKLY
classification. Option.ExpiryID references it; Option.Expiry is resolved by a join.
For an existing database, follow the [expiry migration instructions](scripts/migrations/README.md)
before running the updated repository queries. Fresh databases use `scripts/lab_schema.sql`.

The configured start/end dates select expiry cycles, inclusively. For each expiry,
the simulation starts 45 calendar days before expiry and continues through expiry.
This can require historical candles before the configured start date. The engine
fetches each day's 5-minute underlying candles in order and skips empty days.
Leg state is recreated for each expiry; EntryDTE on OptionLeg is not used.

OffsetSelector calculates the nearest ATM strike from an already available
underlying candle's Open, rounded to INR 50 intervals (halfway rounds up), then
applies its signed StrikeOffset. It returns an integer-paise strike and has no
repository, timestamp validation, or contract resolution responsibilities.

```go
var strikeSelector models.StrikeSelector = selector.NewOffsetSelector(2)
strike, err := strikeSelector.SelectStrike(candle)
```

PremiumSelector and DeltaSelector return explicit not-implemented errors.

Scripts define `initialize(ctx)` and `on_candle(ctx)`. The script is compiled once,
initialized once, and its globals are frozen. Configuration is permitted only
during initialization. Legs are created on their first entry request, with quantity
specified in that call. The following strategy sells a call two
INR 50 strike intervals above ATM at 45 DTE and requests closure at 21 DTE:

```python
def initialize(ctx):
    ctx.configure(underlying="NIFTY", start_date="2025-01-01", end_date="2025-12-31")

def on_candle(ctx):
    if ctx.dte == 45:
        ctx.sell_call("CE", quantity=75, offset=2)
    if ctx.dte == 21:
        ctx.close("CE")
```

`ctx.dte` counts calendar days. `ctx.underlying_candle` exposes `open`, `high`,
`low`, `close` in paise and `start_time` as an RFC3339 timestamp. `ctx.leg("CE")`
and `ctx.legs["CE"]` provide read-only snapshots with `status`, `selected_contract`,
`entry_price`, `quantity`, and `option_candle`. Unavailable values are `None`.
The selected contract exposes `id`, `strike_price` in paise, `expiry`, and
`option_type`. The option candle has the same fields as the underlying candle.

Callbacks observe completed underlying candles. Orders requested by a callback
fill at the open of the next available underlying timestamp's matching option
candle, never at the already-observed candle's open. Thus a 09:15 candle's OHLC is
observed at 09:20, and its order can fill at the 09:20 option candle's open.
Each open leg's current option candle is updated before the callback. Exact
timestamp matching is required: missing option data fails the backtest explicitly,
without forward-filling or looking ahead. Requested orders are queued until a
subsequent candle; unfilled orders at expiry return an error.

Each expiry starts with an empty leg collection. Repeated entries for an existing
leg are no-ops and preserve its original quantity. Repeated closes for non-open
legs are no-ops. Looking up or closing a name before its first entry request is an
error; use `"CE" in ctx.legs` to check existence. There is no re-entry, rolling, adjustment, forced
expiry closure, fee model, or slippage model. Closed short trades record gross
PnL as `(entry_price - exit_price) * quantity` in paise. Open positions remain in
the returned trade histories. If no candle exists on exactly 45 or 21 DTE, that
equality condition does not trigger on another day.

Run checks with `go test ./...` and `go vet ./...`.

Set `DATABASE_URL` to the PostgreSQL connection string and run:

```powershell
go run . -strategy strategies/short_call.star
```

Edit that script to change the underlying, dates, quantity, offset, or conditions.
The database must already contain the underlying, classified expiry cycles,
and historical 5-minute underlying candles. Underlying ingestion remains manual.
Option contracts already in PostgreSQL are returned directly. When a contract
is missing, the repository can synchronously request its ingestion from the
Python data fetcher, then query PostgreSQL again.

Start the [Python service](data-fetcher/README.md) and set its base URL before
starting Go:

```powershell
$env:DATA_FETCHER_URL = "http://127.0.0.1:8000"
```

`DATA_FETCHER_URL` is read when the repository is constructed; it is only required
on a contract miss. The reusable HTTP client has a 30-minute timeout, with earlier
cancellation/deadlines supported through the caller's context. Redirects are not
followed and there are no application retries.

The request is `POST /options/fetch` with `underlying`, `expiry`, `strike`, and
`option_type`. Strikes are converted from integer paise to an exact JSON decimal
in rupees (2500005 paise becomes 25000.05). Python must return a synchronous 2xx
JSON response with status `ingested` or `already_exists` and a positive
`contract_id`. Go then re-runs the original contract query rather than trusting
the response as contract data. Missing data after successful ingestion, HTTP
failures, and database errors are propagated; an existing contract never makes
an HTTP request.

Both processes must use the same database/schema. Python owns all ingestion
rules and writes, including fixed 45-DTE/5-minute history and atomic commits.
Its existing expiry-catalog and historical-data restrictions still apply.
Go never calls `/underlyings/fetch`.
Use `postgres://localhost:5432/your_database` to select a database explicitly.
Keyword/value connection strings are also supported:

```powershell
$env:DATABASE_URL = "host=localhost port=5432 user=postgres password=password dbname=optionsbacktest sslmode=disable"
```

Include the username and password in `DATABASE_URL`.
The application verifies connectivity, loads the script, and runs the backtest.

The repository logs fetch diagnostics at DEBUG. The application logs completion
or errors once. No database schema or Python ingestion changes are required.
