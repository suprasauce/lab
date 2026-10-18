# Options backtesting models

Minimal Go models with no external dependencies.

- `models`: underlying, instrument variants, strategy, leg, trade, and candle.
- `engine`: package placeholder only.

Underlying-to-instrument relationships use `UnderlyingID`. Strategies contain
legs, legs reference instruments and contain trades, and trades identify their
leg and source execution. IDs are opaque strings; persistence can choose their
format later. No candle ownership or persistence mapping is defined yet.

Prices use int64 amounts in paise (100 paise = 1 rupee). Convert rupees to
paise before assigning prices. Quantities count units rather than lots.

Models contain no validation or calculation logic. Instrument variants retain
only their `Type` methods to implement the `Instrument` interface.

Run checks with `go test ./...` and `go vet ./...`.
