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
	StrikePrice  Price
	Expiry       time.Time
	OptionType   OptionType
}
