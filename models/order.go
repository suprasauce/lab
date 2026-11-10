package models

import (
	"fmt"
	"time"
)

type OrderStatus string

const (
	OrderPending OrderStatus = "PENDING"
	OrderFilled  OrderStatus = "FILLED"
)

// Order retains placement and execution details; selectors live in the engine.
type Order struct {
	ID        string
	Side      string
	Quantity  int64
	PlacedAt  time.Time
	Status    OrderStatus
	Contract  *Option
	FilledAt  time.Time
	FillPrice Price
}

// GenerateID assigns an ID scoped to the expiry and order sequence.
func (o *Order) GenerateID(expiry time.Time, sequence int) {
	o.ID = fmt.Sprintf("%s/%d", expiry.Format(time.DateOnly), sequence)
}

// Validate checks order inputs without resolving contracts or inspecting positions.
func (o Order) Validate() error {
	if o.Side != "BUY" && o.Side != "SELL" {
		return fmt.Errorf("invalid side %q: expected BUY or SELL", o.Side)
	}
	if o.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	return nil
}
