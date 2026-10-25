# Options backtesting models

Minimal Go models and a PostgreSQL market data repository.

- `models`: underlying, option, expiry, strategy, leg, trade, and candle.
- `engine`: backtesting engine skeleton; evaluation and execution are unimplemented.
- `selector`: strike selection, with offset selection implemented and premium/delta placeholders.
- `database`: reads DATABASE_URL and initializes and verifies the PostgreSQL pool using pgx.
- `repository`: market data queries using an injected database/sql pool.
- `logging`: configurable slog JSON logger writing to stdout; INFO by default in main.
- `main.go`: initializes the pool and repository and closes the pool before returning.

Underlying-to-instrument relationships use `UnderlyingID`. Strategies retain run
only ID and name metadata, without an underlying ID, dates, or a leg collection.
OptionLeg and LegExecution remain standalone models for future script-driven
configuration and runtime state. Each trade stores a position's
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

OptionLeg retains its ID, position side, quantity, OptionType (CALL/PUT), and EntryDTE.
EntryDTE is no longer used by the simulation clock. Strategy-specific entry and exit
times and selector configuration are no longer stored on the leg.
For each expiry, the engine starts at the supplied start date and
queries each calendar day's 5-minute underlying candles through expiry, inclusive.
It iterates days and candles, without initializing or processing leg executions.
The supplied start and end dates also select expiries. No selector calls or trade
execution occur yet.

OffsetSelector calculates the nearest ATM strike from an already available
underlying candle's Open, rounded to INR 50 intervals (halfway rounds up), then
applies its signed StrikeOffset. It returns an integer-paise strike and has no
repository, timestamp validation, or contract resolution responsibilities.

```go
var strikeSelector models.StrikeSelector = selector.NewOffsetSelector(2)
strike, err := strikeSelector.SelectStrike(candle)
```

PremiumSelector and DeltaSelector return explicit not-implemented errors.

`engine.Engine.Backtest` remains the execution entry point and processes option
expiries sequentially. It accepts the underlying ID and start and end dates as
explicit arguments rather than strategy fields, ready for a future script caller.
The application currently only initializes dependencies and
checks database connectivity; it no longer constructs or runs a hardcoded strategy.
Starlark integration is not implemented yet.

Run checks with `go test ./...` and `go vet ./...`.

Set `DATABASE_URL` to the PostgreSQL connection string and run `go run .`.
Use `postgres://localhost:5432/your_database` to select a database explicitly.
Keyword/value connection strings are also supported:

```powershell
$env:DATABASE_URL = "host=localhost port=5432 user=postgres password=password dbname=optionsbacktest sslmode=disable"
```

Include the username and password in `DATABASE_URL`.
The application verifies connectivity and exits.

The repository logs fetch diagnostics at DEBUG. Successful operations and
application lifecycle events are not logged. Returned errors are left for callers
to handle and log once. Initialization
failure logs omit raw driver errors because they may contain connection details.
