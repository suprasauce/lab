package models

import "time"

type StrategyType string

const (
	StrategyOptions StrategyType = "OPTIONS"
	StrategyEquity  StrategyType = "EQUITY"
)

type Strategy struct {
	UnderlyingID int64
	ID           string
	Name         string
	Type         StrategyType
	// StartDate and EndDate are inclusive calendar-date boundaries for expiry selection.
	StartDate time.Time
	EndDate   time.Time
	Legs      []Leg
}
