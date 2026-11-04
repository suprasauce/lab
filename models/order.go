package models

import (
	"fmt"
	"time"
)

// ContractRequest identifies a known contract or describes ATM selection at execution.
// Exactly one of ContractID and OptionType must be supplied.
type ContractRequest struct {
	ContractID int64
	OptionType OptionType
	Offset     int
}

type OrderStatus string

const (
	OrderPending OrderStatus = "PENDING"
	OrderFilled  OrderStatus = "FILLED"
)

// Order retains the original request and its eventual execution details.
type Order struct {
	ID         string
	Request    ContractRequest
	Side       string
	Quantity   int64
	PlacedAt   time.Time
	Status     OrderStatus
	Contract   *Option
	FilledAt   time.Time
	FillPrice  Price
	SelectedAt time.Time
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
	if o.Request.ContractID != 0 {
		if o.Request.ContractID < 0 || o.Request.OptionType != "" || o.Request.Offset != 0 {
			return fmt.Errorf("contract must be a positive ID or an ATM option specification")
		}
	} else if o.Request.OptionType != Call && o.Request.OptionType != Put {
		return fmt.Errorf("invalid option_type %q: expected CALL or PUT", o.Request.OptionType)
	}
	return nil
}
