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

// Order retains the requested trading symbol and placement/execution details.
type Order struct {
	TradingSymbol string
	ID            string
	Side          string
	Quantity      int64
	PlacedAt      time.Time
	Status        OrderStatus
	Contract      *Option
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
	if o.Side != "BUY" && o.Side != "SELL" {
		return fmt.Errorf("invalid side %q: expected BUY or SELL", o.Side)
	}
	if o.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	return nil
}
