# Options backtesting models

Minimal Go models and a PostgreSQL market data repository.

- `models`: underlying, instrument variants, strategy, leg, trade, and candle.
- `engine`: backtesting engine skeleton; evaluation and execution are unimplemented.
- `selector`: option contract selection, with offset selection implemented and premium/delta placeholders.
- `database`: reads DATABASE_URL and initializes and verifies the PostgreSQL pool using pgx.
- `repository`: market data queries using an injected database/sql pool.
- `logging`: configurable slog JSON logger writing to stdout; INFO by default in main.
- `main.go`: initializes the pool and repository and closes the pool before returning.

Underlying-to-instrument relationships use `UnderlyingID`. Strategies contain
legs as concrete OptionLeg values containing their ID, position side, quantity,
and option configuration. LegExecution points to an OptionLeg and associates it with
fresh runtime state for each expiry. Trades identify their leg and source execution.
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

OptionLeg configures OptionType (CALL/PUT), EntryDTE, EntryHour, EntryMinute,
ExitDTE, ExitTime, and an InstrumentSelector. Entry clock components use
Asia/Kolkata, loaded once by the engine; ExitTime retains HH:mm format.
Leg configuration is assumed to be validated before backtesting.
The engine calculates the target entry timestamp and delegates contract selection.
OffsetSelector fetches the first available 1-minute underlying candle, validates
that it is between the target timestamp and the end of the Kolkata expiry date,
and rounds its open price to the nearest INR 50 strike (halfway rounds up).
Its signed StrikeOffset counts intervals above or below ATM. It then resolves
the option contract. LegState stores the target timestamp, selected contract,
and strike; status remains PENDING. Trade execution is not implemented.

Construct selectors with the same repository passed to the engine:

```go
backtestEngine := engine.NewEngine(marketDataRepository)
leg.InstrumentSelector = selector.NewOffsetSelector(marketDataRepository, 2)
```

PremiumSelector and DeltaSelector return explicit not-implemented errors.

`engine.Engine.Run` accepts a strategy and historical candles already obtained
for its time range. TODOs outline chronological processing and independent leg
evaluation. V1 will allow one full entry and one full exit per leg, with no
re-entry. Run currently returns an explicit not-implemented error.

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
