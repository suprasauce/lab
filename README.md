# Options backtesting models

Minimal Go models and a PostgreSQL market data repository.

- `models`: underlying, instrument variants, strategy, leg, trade, and candle.
- `engine`: backtesting engine skeleton; evaluation and execution are unimplemented.
- `database`: reads DATABASE_URL and initializes and verifies the PostgreSQL pool using pgx.
- `repository`: market data queries using an injected database/sql pool.
- `logging`: configurable slog JSON logger writing to stdout; INFO by default in main.
- `main.go`: initializes the pool and repository and closes the pool before returning.

Underlying-to-instrument relationships use `UnderlyingID`. Strategies contain
legs, legs reference instruments and contain trades, and trades identify their
leg and source execution. Underlying IDs and option contract IDs use int64 to match PostgreSQL BIGINT;
other IDs currently remain strings. No candle ownership mapping is defined yet.

Prices use int64 amounts in paise (100 paise = 1 rupee). Convert rupees to
paise before assigning prices. Quantities count units rather than lots.

Models contain no validation or calculation logic. Instrument variants retain
only their `Type` methods to implement the `Instrument` interface.

Each leg has a configured quantity, entry criteria, exit criteria, and an
instrument selector. Entry criteria decide when to enter; selectors decide what
to trade. No concrete criteria or selectors are implemented.

`engine.Engine.Run` accepts a strategy and historical candles already obtained
for its time range. TODOs outline chronological processing and independent leg
evaluation. V1 will allow one full entry and one full exit per leg, with no
re-entry. Run currently returns an explicit not-implemented error.

Run checks with `go test ./...` and `go vet ./...`.

Set `DATABASE_URL` to the PostgreSQL connection string and run `go run .`.
The application verifies connectivity and exits.

The repository logs fetch diagnostics at DEBUG. Successful operations and
application lifecycle events are not logged. Returned errors are left for callers
to handle and log once. Initialization
failure logs omit raw driver errors because they may contain connection details.
