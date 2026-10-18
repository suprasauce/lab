package models

import "time"

type StrategyType string

const (
	StrategyOptions StrategyType = "OPTIONS"
	StrategyEquity  StrategyType = "EQUITY"
)

type Strategy struct {
	ID        string
	Name      string
	Type      StrategyType
	StartTime time.Time
	EndTime   time.Time
	Legs      []Leg
}
