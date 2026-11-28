package models

import (
	"fmt"
	"time"
)

// Trade records one fill belonging to a simulation's position for a trading symbol.
// Quantity is signed lots for options and units for underlyings.
type Trade struct {
	ID            string
	SimulationID  string
	TradingSymbol string
	Timestamp     time.Time
	Price         Price
	Quantity      int64
}

func (t Trade) Validate() error {
	if t.TradingSymbol == "" {
		return fmt.Errorf("tradingsymbol must be nonempty")
	}
	if t.Quantity == 0 {
		return fmt.Errorf("quantity must be nonzero")
	}
	if t.Price < 0 {
		return fmt.Errorf("price must be nonnegative")
	}
	return nil
}
