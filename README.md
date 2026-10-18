# Options backtesting models

Minimal Go models with no external dependencies.

- `models`: underlying, instrument variants, strategy, leg, trade, and candle.
- `engine`: backtesting engine skeleton; evaluation and execution are unimplemented.

Underlying-to-instrument relationships use `UnderlyingID`. Strategies contain
legs, legs reference instruments and contain trades, and trades identify their
leg and source execution. IDs are opaque strings; persistence can choose their
format later. No candle ownership or persistence mapping is defined yet.

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
