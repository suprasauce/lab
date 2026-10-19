package models

import (
	"context"
	"time"
)

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
