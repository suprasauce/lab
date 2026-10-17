package models

import "time"

type TradeSide string

const (
	Buy  TradeSide = "BUY"
	Sell TradeSide = "SELL"
)

// Trade can allocate part of an execution to a leg. Allocations from the same
// execution share ExecutionID, but each has its own ID and Quantity.
type Trade struct {
	ID          string
	ExecutionID string
	LegID       string
	Side        TradeSide
	Price       Price
	Quantity    int64
	Timestamp   time.Time
}
