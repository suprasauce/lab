# Options backtesting

Go execution engine with Starlark strategy definitions and PostgreSQL market data.

- `models`: underlying, option, expiry, strategy, backtest, trade, position side, and candle.
- `engine`: expiry simulation clock, option orders, position state, and trade history.
- `scripting`: Starlark loading, initialization, context bindings, and candle callbacks.
- `strategies`: strategy scripts, including the 45-DTE entry / 21-DTE exit example.
- `selector`: strike selection, with offset selection implemented and premium/delta placeholders.
- `database`: reads DATABASE_URL and initializes and verifies the PostgreSQL pool using pgx.
- `repository`: market data and backtest persistence using an injected database/sql pool.
- `worker`: one background worker consuming queued backtest IDs.
- `logging`: configurable slog JSON logger writing to stdout; INFO by default in main.
- `main.go`: initializes PostgreSQL, the shared repository, worker pool, and Gin HTTP server.

Underlying-to-instrument relationships use `UnderlyingID`. The optional Strategy
metadata model contains only ID and name. The script supplies all run configuration.
The engine keeps independent order and position histories for each expiry.
Each trade stores a position's
contract, entry and exit details, side, quantity, status, and P&L field.
Underlying IDs and option contract IDs use int64 to match PostgreSQL BIGINT;
other IDs currently remain strings. No candle ownership mapping is defined yet.

Prices use int64 amounts in paise (100 paise = 1 rupee). Convert rupees to
paise before assigning prices. Quantities count units rather than lots.

Option and Underlying are concrete market-data models; no generic instrument
interface is required.

OptionExpiry stores each underlying's expiry date and explicit MONTHLY/WEEKLY
classification. Option.ExpiryID references it; Option.Expiry is resolved by a join.
For an existing database, follow the [expiry migration instructions](scripts/migrations/README.md)
before running the updated repository queries. Fresh databases use `scripts/lab_schema.sql`.

The configured start/end dates select expiry cycles, inclusively. For each expiry,
the simulation starts 45 calendar days before expiry and continues through expiry.
This can require historical candles before the configured start date. The engine
advances the simulation clock through sessions identified from available underlying
data. Order, position, and script state are recreated for each expiry.

OffsetSelector calculates the nearest ATM strike from an already available
underlying candle's Open, rounded to INR 50 intervals (halfway rounds up), then
applies its signed StrikeOffset. It returns an integer-paise strike and has no
repository, timestamp validation, or contract resolution responsibilities.

```go
var strikeSelector models.StrikeSelector = selector.NewOffsetSelector(2)
strike, err := strikeSelector.SelectStrike(candle)
```

PremiumSelector and DeltaSelector return explicit not-implemented errors.

Scripts define `initialize(ctx)` and `on_candle(ctx)`. Scripts are compiled and
initialized once. Globals stay frozen; mutable per-expiry values belong in
`ctx.state`. `strategies/short_strangle.star` submits a short call at ATM + 2 and a short put at ATM - 2,
then fully closes both at 21 DTE:

```python
def initialize(ctx):
    ctx.configure(
        underlying="NIFTY",
        start_date="2026-01-01",
        end_date="2026-06-30",
    )


def on_candle(ctx):
    if ctx.dte <= 45 and ctx.time >= "09:30":
        if "call_entry_order_id" not in ctx.state:
            ctx.state["call_entry_order_id"] = ctx.place_order(
                contract=atm_option("CALL", offset=2),
                side="SELL",
                quantity=75,
            )

        if "put_entry_order_id" not in ctx.state:
            ctx.state["put_entry_order_id"] = ctx.place_order(
                contract=atm_option("PUT", offset=-2),
                side="SELL",
                quantity=75,
            )

    if ctx.dte <= 21 and ctx.time >= "15:15":
        if "call_entry_order_id" in ctx.state and "call_exit_order_id" not in ctx.state:
            entry = ctx.order(ctx.state["call_entry_order_id"])
            if entry.status == "FILLED":
                ctx.state["call_exit_order_id"] = ctx.place_order(
                    contract=entry.contract_id,
                    side="BUY",
                    quantity=75,
                )

        if "put_entry_order_id" in ctx.state and "put_exit_order_id" not in ctx.state:
            entry = ctx.order(ctx.state["put_entry_order_id"])
            if entry.status == "FILLED":
                ctx.state["put_exit_order_id"] = ctx.place_order(
                    contract=entry.contract_id,
                    side="BUY",
                    quantity=75,
                )
```

`ctx.place_order(contract, side, quantity)` returns a new string order ID on every
call. Side must be `BUY` or `SELL`, and quantity must be a positive integer.
`contract` accepts either a positive database contract ID or
`atm_option("CALL", offset=2)` / `atm_option("PUT", offset=-2)`. Offset defaults
to zero and counts INR 50 intervals above or below nearest ATM. The helper only
creates an immutable specification; it performs no market-data lookup.

Orders are queued independently in submission order. There are no logical legs
or implicit duplicate suppression. Use `ctx.state` to submit once. The dictionary
persists across callbacks within an expiry and starts completely empty for each
new expiry. Values assigned during `initialize` are not carried into expiry state.
State uses standard Starlark dictionary rules, with no additional restrictions on
keys or values; nested dictionaries, lists, tuples, and other values are allowed. Orders and positions are also independent per expiry.
Order IDs are unique within a backtest, not persistent across separate runs.

`ctx.order(order_id)` returns a read-only snapshot with `id`, `status` (`PENDING`
or `FILLED`), and `contract_id` (`None` until resolved). Use a filled entry's
contract ID for the exit so that a changed ATM does not select another contract.
Unknown order IDs are errors. No position-inspection scripting API is exposed.
The context exposes `configure`, `place_order`, `order`, `state`, `time`, and `dte`.

`ctx.time` is the Asia/Kolkata simulation time as a zero-padded 24-hour `HH:mm`
string. Use comparisons such as `ctx.time >= "09:30"`. DTE counts calendar days.
The clock advances every five minutes from 09:15 through 15:30. Strategy callbacks
stop at 15:25; the final 15:30 advance processes orders without a new callback.
Session existence is determined by PostgreSQL underlying candles, with no holiday
calendar. Each expiry starts 45 calendar days before expiry; configured dates
select expiry cycles rather than restricting these earlier simulation days.

All selection and execution happens on the next advance, before the next script
callback. An ATM request placed at 09:20 is processed at 09:25 using the completed
09:20-09:25 underlying candle's Open. Strike selection reuses that loaded candle.
The option candle's Open supplies the fill price, so the actual execution timestamp
can be 09:20 even though processing occurs at 09:25. Exact contract IDs skip strike
selection and are resolved at execution; they must belong to the current underlying
and expiry. A minimal repository lookup resolves IDs not already in position history.

Only completed OHLC snapshots are loaded, and snapshots are cleared on each tick.
At 09:25 the completed bar starts at 09:20. At 09:15 no same-session candle has
completed. Missing required underlying or open-position completed candles still
stop the backtest with an explicit error. Pending entries can wait for available
option candles without reselecting their resolved contract. Fill prices use the
first eligible completed option candle Open: at 09:20, the latest eligible candle
starts at 09:15. If it is missing, the order waits; the 09:20 candle is not eligible
until 09:25. Neither placement nor selection timestamps are backdated.
Unfilled orders remaining at expiry produce an error.

Positions retain exact contract metadata and reuse the existing Trade entry/exit
fields and integer-paise prices. SELL opens SHORT and BUY opens LONG when no
position is open in that contract. An opposite-side order must match the entire
open quantity to close it; the exit reuses its loaded option candle. Scaling,
partial exits, and reversals are rejected at execution. P&L is `(entry - exit) *
quantity` for SHORT and `(exit - entry) * quantity` for LONG. Closed positions stay
in history; another order is a new request, so scripts must manage their intent.
CSV output keeps its existing columns and includes open and closed positions.
No stop losses, adjustments, forced expiry closure, fees, or slippage are added.

Run checks with `go test ./...` and `go vet ./...`.

Set `DATABASE_URL` to the PostgreSQL connection string and start the HTTP server:

```powershell
go run .
```

Submit an existing script with a numeric user ID that already exists in `lab.users`:

```powershell
$body = @{ script = [System.IO.File]::ReadAllText((Resolve-Path "strategies/short_call.star").Path) } | ConvertTo-Json -Compress
$body | curl.exe -X POST "http://localhost:8080/api/backtests" -H "Content-Type: application/json" -H "X-User-ID: 1" --data-binary "@-"
```

Use `strategies/short_strangle.star` instead to trade both the call and put.
The API inserts a `PENDING` backtest and returns `202` with its string database ID.
One worker consumes IDs from the in-memory queue, fetches the stored script, and
loads it directly from source through the existing Starlark engine. Work is
independent of the original HTTP request's lifetime.

A successful run saves `{"summary": {"total_trades": ..., "total_pnl": ...},
"trades": [...]}` in `lab.backtests.results` and sets `COMPLETED` in the same
update. Script or execution failures save `{"error": "..."}` and set `FAILED`.
Updates also set `updated_at`. Fetch/persistence failures are logged with the
backtest ID, and the worker continues with the next queued job. There is no
`RUNNING` state, automatic retry, or recovery of queued IDs after restart.
Existing PENDING rows are not automatically submitted when the server starts.

Stored JSON uses the CSV fields, exact rupee decimals, and IST timestamps. Open
positions have null exit fields and P&L; summary P&L sums closed positions only.
The optional `utils.WriteBacktestCSV(io.Writer, *engine.BacktestResult)` exporter
retains its existing columns. The worker persists JSON and does not write CSV files.

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
