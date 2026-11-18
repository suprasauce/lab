package models

import "time"

// Trade records one fill belonging to a simulation's position for a trading symbol.
type Trade struct {
	ID            string
	SimulationID  string
	TradingSymbol string
	Timestamp     time.Time
	Price         Price
	Quantity      int64
}
