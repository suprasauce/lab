package models

// EntryCriteria decides when a leg should enter, independently of instrument selection.
type EntryCriteria interface {
	ShouldEnter(candle Candle, leg Leg) (bool, error)
}

// ExitCriteria decides when an open leg should exit in full.
type ExitCriteria interface {
	ShouldExit(candle Candle, leg Leg) (bool, error)
}

// InstrumentSelector chooses what to trade when entry is triggered.
// Future implementations may select options by offset, premium, or delta.
// TODO: Define the market context needed by selectors when implementing them.
type InstrumentSelector interface {
	Select(candle Candle) (Instrument, error)
}
