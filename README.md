# Options Backtesting

An options backtesting service built with Go, Starlark, and PostgreSQL. Starlark scripts define trading rules; Go retrieves market data, simulates orders and positions, and stores the results as JSON.

## Architecture

```text
HTTP request ? Controller ? Service ? Repository ? PostgreSQL
                              ?
                         In-memory queue
                              ?
                        Background worker
                              ?
                    Starlark script + Go engine
                              ?
                   Results saved to PostgreSQL
```

- **Controllers** validate HTTP requests and return responses.
- **Services** coordinate submission and retrieval of backtests.
- **Repository** contains database operations, with SQL kept in `repository/queries.go`.
- **Worker** processes queued backtest IDs. There is currently one worker.
- **Scripting** connects Starlark functions to the Go engine.
- **Engine** manages the simulation clock, orders, positions, and P&L.
- **Python data fetcher** independently ingests market data from ICICI Breeze into the same database.

## How a request is processed

1. The client submits a Starlark script to `POST /api/backtests`, with a numeric `X-User-ID` header.
2. The API validates the request, inserts a `PENDING` backtest in PostgreSQL, and queues its ID.
3. The API returns `202 Accepted` with the backtest ID without waiting for execution.
4. The worker retrieves the saved script, compiles it, and calls `initialize(ctx)` once. The script calls `ctx.configure()`, retrieves expiries for explicit underlyings, and simulates the chosen dates.
5. Each `ctx.simulate(start_date=date.add(expiry.date, -45), end_date=expiry.date)` call runs synchronously in Go, invoking `on_candle(ctx)` on a separate Starlark callback thread. Results are aggregated in the order the script runs the expiries.
6. After execution, the worker saves the summary and trades as JSON and sets the status to `COMPLETED`. Script or execution errors are saved with status `FAILED`.
7. The client retrieves status and results using `GET /api/backtests/{id}`, or lists its backtests using `GET /api/backtests`.

Both GET endpoints use `X-User-ID` to restrict results to that user. This header is the current user-identification mechanism; authentication is not implemented.

## Script-controlled expiry runs

```python
def initialize(ctx):
    ctx.configure()
    for expiry in ctx.get_expiries(underlying="NIFTY", start="2026-01-01", end="2026-06-30"):
        ctx.clear_state()
        ctx.simulate(start_date=date.add(expiry.date, -45), end_date=expiry.date)


def on_candle(ctx):
    # Existing place_trade/state APIs remain available here.
    pass
```

`get_expiries` returns chronological expiry objects with `.date`, `.underlying`,
and `.get_dte(ctx.date)`, with inclusive date boundaries.
`simulate(start_date, end_date)` accepts inclusive YYYY-MM-DD boundaries; it does not require an expiry or underlying. `date.add(date, days)` shifts a date by calendar days. `configure()` takes no arguments.
Each returned trade uses its contract's underlying symbol, allowing different
underlyings across expiry simulations. The script chooses which dates to simulate and in what order.
Discovery and simulation are allowed only during initialization, after configure.
Call `ctx.clear_state()` before each simulation when independent script state is
needed, then assign values such as `ctx.state["expiry"] = expiry.date`. The dictionary
is cleared in place, so existing references see the cleared contents too.
Existing saved scripts must move date ranges out of `configure` and explicitly
call `simulate`; there is no automatic expiry loop. `ctx.time` remains an HH:mm
string. No close-all API is added.

## How the simulation works

Each simulation runs from its supplied start date through its end date, inclusive. The script chooses the range; for an expiry-based strategy, it can start 45 calendar days before expiry using `date.add(expiry.date, -45)`.

The clock advances in five-minute intervals from 09:15 through 15:30 in Asia/Kolkata. Available underlying data determines which days have trading sessions. At each advance, the engine loads completed candles and processes pending orders before calling the script's `on_candle(ctx)`. The final 15:30 advance processes orders without another script callback.

Scripts access the current minute's full OHLC with `ctx.get_candle(tradingsymbol)`, returning `None` when that minute is missing. At 09:19 this exposes the 09:19?09:20 candle. Scripts are responsible for the chronological use of candle values; the engine does not enforce intrabar lookahead restrictions.

`ctx.place_trade(tradingsymbol, quantity, price)` executes immediately at the supplied rupee price and records the current simulation timestamp. Use `candle.open`, `.high`, `.low`, `.close`, or a numeric expression. The call returns the recorded trade ID, and position updates are visible to subsequent statements in the same callback. There is no pending-order queue or fill policy. Unknown instruments and unsupported position changes return errors. The engine marks unrealized P&L only when a candle finishes: at 09:18 it uses the 09:17 candle Close while exposing the 09:18 candle to the script. New or reopened positions start with zero unrealized P&L. Missing completed candles retain the previous value and add no history point; missing current candles expose `None` without preventing a valid previous candle from being marked. The final 15:29 candle is marked at 15:30, without a script callback.

```python
candle = ctx.get_candle("NIFTY")
if candle != None:
    trade_id = ctx.place_trade(tradingsymbol="NIFTY", quantity=1, price=candle.open)
```

Trades accept an opaque `tradingsymbol` string. At execution time, the repository
queries the stored `tradingsymbol` directly. Missing instruments return errors. Missing candles return `None`; scripts decide whether to execute at an explicitly supplied price. Database errors propagate.
Go never calls the Python fetcher. Option-chain selection requires the expiry's
`option_chain_fetched` flag to be true; otherwise it returns "option chain not available".
Ingest market data separately before running a backtest.

Trade history and positions start fresh for every simulation. `ctx.state` persists
across expiry simulations until the script calls `ctx.clear_state()`. Each new
backtest starts with an empty state dictionary. Prices are stored internally as integer paise and returned in JSON as rupee amounts. Results contain a summary, positions, and individual fills.

## Lab option trading symbols

Lab uses its own format, not NSE, Zerodha, or Breeze broker symbols:

```text
UNDERLYING-YYYYMMDD-STRIKE-CE/PE
```

- `UNDERLYING`: uppercase letters/digits, beginning with a letter, such as NIFTY or BANKNIFTY.
- `YYYYMMDD`: exact expiry calendar date, with no expiry-calendar inference.
- `STRIKE`: positive integer rupee value with no leading zeros.
- `CE` means CALL; `PE` means PUT. Weekly and monthly expiries use identical formatting.

Examples: `NIFTY-20261124-26000-CE`, `NIFTY-20261124-26000-PE`,
`NIFTY-20261201-26500-CE`, and `BANKNIFTY-20261230-60000-CE`.
An expiry classified as monthly and an expiry classified as weekly both encode
their full stored date; that classification is not encoded in the symbol.
Strikes use integer rupees: 2600000 paise becomes `26000`. Any remaining
paise are discarded, so 2600050 also becomes `26000`.

```python
ctx.place_trade(
    tradingsymbol="NIFTY-20261124-26000-CE",
    quantity=-1,
    price=100.50,
)
```

No `ctx.option_chain()` call is needed to construct a symbol. A syntactically valid
symbol does not establish that its contract exists: the database must contain it.
The Go parser lives in `models/option_symbol.go`; contract creation uses the
Python generator in `data-fetcher/trading_symbol.py`. Both use integer paise.

The resolver always queries `lab.option_contracts.tradingsymbol` directly.
Migration `010_option_contract_tradingsymbol.sql` adds and backfills this column.
There is no schema-detection query or missing-symbol parsing fallback.
Lookups are database-only; Go does not trigger ingestion.


## Market data

The database must already contain the underlying, its classified expiry dates, and historical five-minute underlying candles. Underlying ingestion is manual.

The independent Python service ingests a strike range and its full 45-DTE
option history atomically using `POST /option-chain/fetch`. It marks `option_expiry.option_chain_fetched` true after committing.
Go only checks this flag: false or a missing expiry returns "option chain not available".
Missing contracts and candles are not downloaded during backtesting.

See the [data fetcher README](data-fetcher/README.md) for ingestion setup. Go and Python must use the same database.

## Running locally

Set `DATABASE_URL` in the process environment. Start the Go server with:

```sh
go run .
```

The server listens on port 8080. API contracts are documented in [spec/openapi.yaml](spec/openapi.yaml). The submitting user must already exist in `lab.users`.

`GET /api/templates` lists saved strategies owned by the backend-configured `TEMPLATE_OWNER_USER_ID`, ordered by name and ID ascending. Set this environment variable to the designated platform user's positive numeric ID. The endpoint requires no user header and returns `500` if the setting is missing or invalid. The owner ID is never taken from client input or included in the response.

The queue is in memory. On startup, the worker reloads all `PENDING` backtest IDs from PostgreSQL, oldest first, and queues them for execution. Interrupted backtests restart from the beginning. There is no `RUNNING` status or automatic retry during normal operation.

Run tests with `go test ./...`.

The simulation exposes `ctx.date` (`YYYY-MM-DD`) and `ctx.time` (`HH:mm`) in IST.
Expiry objects expose `.date`, `.underlying`, and `.get_dte(ctx.date)` for calendar-day DTE; `ctx.dte` is no longer available.

Trades use signed quantities: `ctx.place_trade(tradingsymbol=symbol, quantity=-1, price=100)` sells,
and `quantity=1` buys. There is no `side` argument. Quantities are signed lots throughout positions, trades, and Starlark. Each fill records its timestamp, per-unit price, and signed lot quantity.
Lot size comes from `option_expiry.lot_size`; realized P&L is `(exit price - average price) * signed lots * lot_size`. Prices remain in rupees per unit.
New expiry rows must specify a positive lot size.
Each simulation has one position per trading symbol, containing its trade history. Positions store simulation ID, signed quantity, average price, cumulative realized P&L, and current candle.
Closing sets quantity to zero and adds realized P&L; reopening reuses the position and resets its average price without clearing P&L or trades.
Simulation IDs (`1`, `2`, ...) restart per backtest. Trade IDs (`1/1`, `1/2`, `2/1`, ...) combine simulation ID and trade sequence.
Results return position summaries and a flat list of fills linked by `simulation_id` and `tradingsymbol`. Summary P&L is the sum of position realized P&L, and total trades counts fills.

During `on_candle`, `ctx.get_positions(tradingsymbol)` returns open and closed positions
for that symbol in the current expiry execution, in creation order. Each read-only snapshot
exposes `simulation_id`, derived `status`, signed `quantity`, `average_price`, `realized_pnl`, and `candle`.
Quantity is lots for options and units for underlyings. Prices are in rupees.
The read-only `candle` exposes `timestamp` (start time in IST), `open`, `high`, `low`, and `close`
for the latest completed 1-minute bar. It is `None` when missing or closed; use
`position.candle.close` when available. No matches returns an empty list.

Each results position includes current `unrealized_pnl` and a timestamped `total_pnl`
history in rupees. Total P&L is cumulative realized plus current unrealized P&L.
After fills, open positions are marked using completed option candle closes and the
expiry lot size. Missing candles skip history points; closed positions record
realized P&L with zero unrealized P&L. Reopening retains realized P&L and history.
The UI plots total P&L; simulation curves sum matching position snapshots.
These P&L fields are not newly exposed to Starlark. No database tables are added.

Scripts can query `ctx.get_lot_size(tradingsymbol)` during `on_candle`: it returns the option expiry's units per lot, or 1 for an underlying. Unknown symbols return an error. `position.unrealized_pnl` is not exposed to scripts; it remains internal and in saved results. To calculate P&L at a script-selected price:

```python
pnl = (price - position.average_price) * position.quantity * ctx.get_lot_size(symbol)
```
