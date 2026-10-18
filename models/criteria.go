package models

import (
	"context"
	"time"
)

// EntryCriteria decides when a leg should enter, independently of instrument selection.
type EntryCriteria interface {
	ShouldEnter(candle Candle, leg Leg) (bool, error)
}

// ExitCriteria decides when an open leg should exit in full.
type ExitCriteria interface {
	ShouldExit(candle Candle, leg Leg) (bool, error)
}

// SelectionInput contains the common market context for option selection.
type SelectionInput struct {
	UnderlyingID int64
	Expiry       time.Time
	EntryTime    time.Time
	OptionType   OptionType
	Timeframe    Timeframe
}

// InstrumentSelector resolves an option contract using its own selection configuration.
type InstrumentSelector interface {
	Select(ctx context.Context, input SelectionInput) (Option, error)
}
