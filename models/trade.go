package models

import "time"

type TradeStatus string

const (
	TradeOpen   TradeStatus = "OPEN"
	TradeClosed TradeStatus = "CLOSED"
)

// Trade records the complete lifecycle of a position in one option contract.
type Trade struct {
	ContractID int64

	Side     PositionSide
	Quantity int64

	EntryTime  time.Time
	EntryPrice Price

	ExitTime  time.Time
	ExitPrice Price

	Status TradeStatus
	PnL    Price
}
