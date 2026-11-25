package models

import (
	"fmt"
	"strconv"
	"time"
)

type OrderStatus string

const (
	OrderPending OrderStatus = "PENDING"
	OrderFilled  OrderStatus = "FILLED"
)

// Order retains the trading symbol, signed quantity, and execution details.
// Quantity is signed lots for options and units for underlyings: positive buys; negative sells.
type Order struct {
	TradingSymbol string
	ID            string
	Quantity      int64
	PlacedAt      time.Time
	Status        OrderStatus
	Contract      *Option
	Underlying    *Underlying
	FilledAt      time.Time
	FillPrice     Price
}

// GenerateID assigns a sequential ID scoped to one execution.
func (o *Order) GenerateID(sequence int) {
	o.ID = strconv.Itoa(sequence)
}

// Validate checks order inputs without resolving contracts or inspecting positions.
func (o Order) Validate() error {
	if o.TradingSymbol == "" {
		return fmt.Errorf("tradingsymbol must be nonempty")
	}
	if o.Quantity == 0 {
		return fmt.Errorf("quantity must be nonzero")
	}
	return nil
}
