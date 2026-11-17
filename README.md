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
5. Each `ctx.simulate(expiry)` call runs synchronously in Go, invoking `on_candle(ctx)` on a separate Starlark callback thread. Results are aggregated in the order the script runs the expiries.
6. After execution, the worker saves the summary and trades as JSON and sets the status to `COMPLETED`. Script or execution errors are saved with status `FAILED`.
7. The client retrieves status and results using `GET /api/backtests/{id}`, or lists its backtests using `GET /api/backtests`.

Both GET endpoints use `X-User-ID` to restrict results to that user. This header is the current user-identification mechanism; authentication is not implemented.

## Script-controlled expiry runs

```python
def initialize(ctx):
    ctx.configure()
    for expiry in ctx.get_expiries(underlying="NIFTY", start="2026-01-01", end="2026-06-30"):
        ctx.clear_state()
        ctx.simulate(expiry)


def on_candle(ctx):
    # Existing place_order/order/state APIs remain available here.
    pass
```

`get_expiries` returns chronological expiry objects with `.date`, `.underlying`,
and `.get_dte(ctx.date)`, with inclusive date boundaries.
`simulate` uses that object's underlying. `configure()` takes no arguments.
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

For each expiry, the simulation starts 45 calendar days before expiry and continues through expiry day. The configured date range selects expiries, so simulation data may be needed before the configured start date.

The clock advances in five-minute intervals from 09:15 through 15:30 in Asia/Kolkata. Available underlying data determines which days have trading sessions. At each advance, the engine loads completed candles and processes pending orders before calling the script's `on_candle(ctx)`. The final 15:30 advance processes orders without another script callback.

Scripts submit orders through `ctx.place_order(...)`. Orders are queued and processed on a subsequent advance. Fills use an eligible completed option candle's Open. For example, an order placed at 09:30 can be processed at 09:35 using the 09:30 candle's Open. The strategy never receives unfinished candle OHLC data.

Orders accept an opaque `tradingsymbol` string. At fill time, the repository
queries the stored `tradingsymbol` directly; the column is required. If no contract
exists, it parses the Lab symbol and reuses full-45-DTE ingestion through the
Python service before continuing. Other database errors are returned immediately. Missing candles leave orders pending
for a later completed five-minute window.

Order history and positions start fresh for every expiry. `ctx.state` persists
across expiry simulations until the script calls `ctx.clear_state()`. Each new
backtest starts with an empty state dictionary. Prices are stored internally as integer paise and returned in JSON as rupee amounts. Results contain a trade summary and position entry/exit details; open positions have null exit fields and P&L.

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
ctx.place_order(
    tradingsymbol="NIFTY-20261124-26000-CE",
    side="SELL",
    quantity=75,
)
```

No `ctx.option_chain()` call is needed to construct a symbol. A syntactically valid
symbol does not establish that its contract exists: the database must contain it.
The Go parser lives in `models/option_symbol.go`; contract creation uses the
Python generator in `data-fetcher/trading_symbol.py`. Both use integer paise.

The resolver always queries `lab.option_contracts.tradingsymbol` directly.
Migration `010_option_contract_tradingsymbol.sql` adds and backfills this column.
There is no schema-detection query. Only a missing symbol triggers parsing and
the existing full-history ingestion flow.


## Market data

The database must already contain the underlying, its classified expiry dates, and historical five-minute underlying candles. Underlying ingestion is manual.

Option ingestion fetches the full 45-DTE history. Missing contracts may be fetched
through `POST /options/fetch`; existing contracts are reused. The synchronous
`POST /option-chain/fetch` endpoint ingests the full strike universe atomically
and records completion in `option_expiry.option_chain_fetched`. `OptionChainService.GetSymbol()`
checks this flag before filter dispatch, synchronously requests full-chain
ingestion when false, and verifies the committed flag before continuing.
Delta, offset, and price selection themselves remain TODOs. Day-only ingestion
and per-contract synchronization flags have been removed.

See the [data fetcher README](data-fetcher/README.md) for ingestion setup. Go and Python must use the same database.

## Running locally

Set `DATABASE_URL` in the process environment. Set `DATA_FETCHER_URL` to the Python service's base URL when on-demand option ingestion is needed. Start the Go server with:

```sh
go run .
```

The server listens on port 8080. API contracts are documented in [spec/openapi.yaml](spec/openapi.yaml). The submitting user must already exist in `lab.users`.

`GET /api/templates` lists saved strategies owned by the backend-configured `TEMPLATE_OWNER_USER_ID`, ordered by name and ID ascending. Set this environment variable to the designated platform user's positive numeric ID. The endpoint requires no user header and returns `500` if the setting is missing or invalid. The owner ID is never taken from client input or included in the response.

The queue is in memory. On startup, the worker reloads all `PENDING` backtest IDs from PostgreSQL, oldest first, and queues them for execution. Interrupted backtests restart from the beginning. There is no `RUNNING` status or automatic retry during normal operation.

Run tests with `go test ./...`.

The simulation exposes `ctx.date` (`YYYY-MM-DD`) and `ctx.time` (`HH:mm`) in IST.
Expiry objects expose `.date`, `.underlying`, and `.get_dte(ctx.date)` for calendar-day DTE; `ctx.dte` is no longer available.

Orders use signed quantities: `ctx.place_order(tradingsymbol=symbol, quantity=-75)` sells,
and `quantity=75` buys. There is no `side` argument. Trades retain their original signed quantity after closing.

During `on_candle`, `ctx.get_positions(tradingsymbol)` returns open and closed positions
for that symbol in the current expiry execution, in creation order. Each read-only snapshot
exposes `status`, signed `quantity`, `entry_price`, `exit_price`, and `curr_price`.
Prices are in rupees. Open positions have `exit_price=None`; `curr_price` is the completed
option candle's Close, or `None` when missing or closed. No matches returns an empty list.
