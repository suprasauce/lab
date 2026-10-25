package models

// StrikeSelector calculates an option strike from an underlying candle.
type StrikeSelector interface {
	SelectStrike(candle Candle) (Price, error)
}
