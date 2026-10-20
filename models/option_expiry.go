package models

import "time"

type ExpiryType string

const (
	ExpiryMonthly ExpiryType = "MONTHLY"
	ExpiryWeekly  ExpiryType = "WEEKLY"
)

type OptionExpiry struct {
	ID           int64
	UnderlyingID int64
	ExpiryDate   time.Time
	ExpiryType   ExpiryType
	CreatedAt    time.Time
}
