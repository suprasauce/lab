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
4. The worker retrieves the saved script, compiles it, and calls `initialize(ctx)` once. The script configures the underlying and backtest dates.
5. The engine resolves the underlying and fetches expiry dates within the configured date range. It processes each expiry sequentially.
6. After execution, the worker saves the summary and trades as JSON and sets the status to `COMPLETED`. Script or execution errors are saved with status `FAILED`.
7. The client retrieves status and results using `GET /api/backtests/{id}`, or lists its backtests using `GET /api/backtests`.

Both GET endpoints use `X-User-ID` to restrict results to that user. This header is the current user-identification mechanism; authentication is not implemented.

## How the simulation works

For each expiry, the simulation starts 45 calendar days before expiry and continues through expiry day. The configured date range selects expiries, so simulation data may be needed before the configured start date.

The clock advances in five-minute intervals from 09:15 through 15:30 in Asia/Kolkata. Available underlying data determines which days have trading sessions. At each advance, the engine loads completed candles and processes pending orders before calling the script's `on_candle(ctx)`. The final 15:30 advance processes orders without another script callback.

Scripts submit orders through `ctx.place_order(...)`. Orders are queued and processed on a subsequent advance. ATM strike selection uses the underlying candle's Open, and fills use an eligible completed option candle's Open. For example, an order placed at 09:30 can be processed at 09:35 using the 09:30 candle's Open. The strategy never receives unfinished candle OHLC data.

The `contract` argument accepts `offset_selector("CALL", offset=2)` (or `"PUT"`) to select a contract at execution time, or `id_selector(entry.contract_id)` to use an existing contract, including for an exit. Both implement the interface in `selector/`; each runtime order holds its selector and resolves its contract once. An offset selector waits when the underlying candle is missing, while an ID selector does not need underlying price data. Fills use only the current completed five-minute candle window; missing candles leave orders pending for a later window without searching older bars. Saved scripts using the old `atm_option(...)` helper or bare contract IDs must be updated to these selector calls.

Order history, positions, and `ctx.state` start fresh for every expiry. Prices are stored internally as integer paise and returned in JSON as rupee amounts. Results contain a trade summary and position entry/exit details; open positions have null exit fields and P&L.

## Market data

The database must already contain the underlying, its classified expiry dates, and historical five-minute underlying candles. Underlying ingestion is manual.

When an option contract is missing, Go synchronously calls the Python service's `POST /options/fetch` endpoint. Python fetches the history and commits the contract and candles together. Go then queries PostgreSQL again. Existing contracts are read directly without an HTTP call.

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
