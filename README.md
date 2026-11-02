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
        ctx.open("CE", option_type="CE", side="SHORT", quantity=75, offset=2)
    if ctx.dte == 21:
        ctx.close("CE")
```

`ctx.time` is the Asia/Kolkata simulation time as a zero-padded 24-hour `HH:mm`
string. Scripts can check `ctx.time >= "09:15"` or `ctx.time >= "15:20"`.
Use quotes and two-digit hours/minutes so string comparisons follow time order.
The clock advances independently of candle gaps, every five minutes from 09:15
through 15:30 inclusive. Strategy callbacks run only through 15:25; the 15:30
advance loads the final completed candles and processes pending orders without
calling the script. A day is simulated only if the database contains at least
one underlying candle starting within [09:15, 15:30). No holiday calendar is used.

`ctx.dte` counts calendar days. The script context exposes only `configure`,
`open`, `close`, `time`, and `dte`. Market candles and leg state remain internal
to the Go engine.

At time T, the engine loads only completed underlying/option candles. At 09:25, the
latest bar is 09:20-09:25, never 09:25-09:30. At 09:15 there is no completed candle
from that session, so candle attributes are `None`. All candle snapshots are cleared
on every simulation tick. A missing required completed underlying candle or an
open leg's completed option candle stops the backtest with an explicit error;
previous candles are never reused. Data is fetched by timestamp as the clock advances,
without preloading a daily candle slice.

`ctx.open(leg_id, option_type, side, quantity, offset=0)` opens a logical leg.
Use `option_type="CE"` for calls or `"PE"` for puts, and `side="LONG"` or
`"SHORT"`. Quantity must be positive; offset counts ?50 strike intervals from
ATM. Existing logical legs are unchanged by repeated open requests. Closing a
LONG position calculates `(exit - entry) ? quantity`; SHORT uses
`(entry - exit) ? quantity`, in integer paise.

`open` and `close` queue market orders with `PlacedAt = ctx.time`. Orders are
processed only inside the next `advance()`, before the next script callback;
there is no post-callback fill pass. The first available option candle with start >= PlacedAt and
start <= simulation time provides the fill Open. An order placed at 09:15 can
record a 09:15 fill when processed at 09:20; a 09:17 placement waits for 09:20. Actual execution times
are recorded separately in Trade.EntryTime/ExitTime. The fill candle's unfinished
High/Low/Close are never published to the script. Position changes are visible
at the next callback, without duplicate fills.

For offset selection, Go reuses the completed underlying candle already loaded
by `advance()` and selects from its Open, without another underlying lookup.
If that snapshot is `None`, the order stays pending for a later advance. A selected
contract is retained while waiting for option data. Missing required completed
candles still produce the errors described above.
Missing option bars also leave orders pending, without filling at earlier opens
or advancing the script into the future. Only regular-session opens are eligible;
15:30 completes the 15:25 bar and is not a new executable candle open. Unfilled
orders carry to the next available session or return an error at expiry.

Each expiry starts with an empty leg collection. Repeated entries for an existing
leg are no-ops and preserve its original quantity. Repeated closes for non-open
legs are no-ops. Looking up or closing a name before its first entry request is an
error. There is no re-entry, rolling, adjustment, forced
expiry closure, fee model, or slippage model. Closed short trades record gross
PnL as `(entry_price - exit_price) * quantity` in paise. Open positions remain in
the returned trade histories. If no candle exists on exactly 45 or 21 DTE, that
equality condition does not trigger on another day.

Run checks with `go test ./...` and `go vet ./...`.

Set `DATABASE_URL` to the PostgreSQL connection string and run:

```powershell
go run . -strategy strategies/short_call.star
```

After a successful backtest, the application writes `results/backtest.csv`,
creating the directory if necessary and overwriting the previous CSV. Columns are
`expiry,underlying,strike,option_type,side,entry_timestamp,exit_timestamp,entry_price,exit_price,quantity,pnl`.
Monetary values are exact rupee decimals; timestamps use RFC3339 with the IST
offset. Each trade has one row. Open trades have blank exit timestamp, exit price,
and realized P&L. A run with no trades writes only the header.

CSV generation lives in `utils.WriteBacktestCSV(io.Writer, *engine.BacktestResult)`;
the engine and scripting packages do not create files or depend on CSV export.
Results retain the existing runtime legs, including their selected contracts and
trade histories, rather than duplicating contract metadata in models.

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
