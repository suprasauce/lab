package models

import "time"

type OptionType string

const (
	Call OptionType = "CALL"
	Put  OptionType = "PUT"
)

type Option struct {
	ID               int64
	UnderlyingID     int64
	UnderlyingSymbol string
	ExpiryID         int64
	LotSize          int64 // Units per lot, joined from option_expiry.
	StrikePrice      Price
	// Expiry is the calendar date joined from option_expiry, not a contract column.
	Expiry     time.Time
	OptionType OptionType
}
