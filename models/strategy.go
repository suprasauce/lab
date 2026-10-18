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
	StartTime    time.Time
	EndTime      time.Time
	Legs         []Leg
}
