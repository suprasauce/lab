package models

import "time"

type InstrumentType string

const (
	InstrumentOption InstrumentType = "OPTION"
	InstrumentFuture InstrumentType = "FUTURE"
	InstrumentEquity InstrumentType = "EQUITY"
)

// Instrument identifies the contract kind without coupling callers to storage.
type Instrument interface {
	Type() InstrumentType
}

type OptionType string

const (
	Call OptionType = "CALL"
	Put  OptionType = "PUT"
)

type Option struct {
	ID           string
	UnderlyingID string
	StrikePrice  Price
	Expiry       time.Time
	OptionType   OptionType
}

func (Option) Type() InstrumentType { return InstrumentOption }

type Future struct {
	ID           string
	UnderlyingID string
	Expiry       time.Time
}

func (Future) Type() InstrumentType { return InstrumentFuture }

type Equity struct {
	ID           string
	UnderlyingID string
}

func (Equity) Type() InstrumentType { return InstrumentEquity }
