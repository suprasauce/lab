package models

import "time"

type OptionType string

const (
	Call OptionType = "CALL"
	Put  OptionType = "PUT"
)

type Option struct {
	ID           int64
	UnderlyingID int64
	ExpiryID     int64
	StrikePrice  Price
	// Expiry is the calendar date joined from option_expiry, not a contract column.
	Expiry     time.Time
	OptionType OptionType
}
