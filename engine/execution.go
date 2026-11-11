package engine

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
	"optionsbacktest/selector"
)

// Position reuses Trade's complete entry/exit lifecycle and price representation.
type Position struct {
	models.Trade
	Contract *models.Option
	Candle   *models.Candle
}

// Execution contains fresh orders and position history for one expiry.
type Execution struct {
	Time             time.Time
	UnderlyingCandle *models.Candle
	DTE              int
	Orders           []*Order
	Positions        []*Position
	ctx              context.Context
	repo             *repository.Repository
	underlyingID     int64
	expiry           time.Time
}

func newExecution(ctx context.Context, repo *repository.Repository, underlyingID int64, expiry time.Time) *Execution {
	return &Execution{ctx: ctx, repo: repo, underlyingID: underlyingID, expiry: expiry}
}

// PlaceOrder always creates a new pending order; it never selects or executes.
func (e *Execution) PlaceOrder(selection selector.Selector, side string, quantity int64) (string, error) {
	if selection == nil {
		return "", fmt.Errorf("a contract selector is required")
	}
	order := &Order{Order: models.Order{Side: side, Quantity: quantity}, Selector: selection}
	if err := order.Validate(); err != nil {
		return "", err
	}
	order.GenerateID(e.expiry, len(e.Orders)+1)
	order.PlacedAt = e.Time
	order.Status = models.OrderPending
	e.Orders = append(e.Orders, order)
	return order.ID, nil
}

func (e *Execution) Order(id string) (*Order, error) {
	for _, order := range e.Orders {
		if order.ID == id {
			return order, nil
		}
	}
	return nil, fmt.Errorf("unknown order %q", id)
}

// advance loads completed bars before filling previously submitted orders.
func (e *Execution) advance(at time.Time) error {
	e.UnderlyingCandle = nil
	for _, position := range e.Positions {
		position.Candle = nil
	}
	e.Time = at
	e.DTE = int(calendarDate(e.expiry).Sub(calendarDate(at)) / (24 * time.Hour))
	if err := e.loadCompletedCandles(); err != nil {
		return err
	}
	return e.fillOrders()
}

func (e *Execution) loadCompletedCandles() error {
	start := e.Time.Add(-candleInterval)
	if startTime, _ := time.Parse(timeLayout, start.Format(timeLayout)); startTime.Before(sessionStartTime) {
		return nil
	}
	// PostgreSQL timestamp precision is one microsecond; this is an exact lookup.
	candles, err := e.repo.GetUnderlyingCandles(e.ctx, e.underlyingID, start, start.Add(time.Microsecond), models.Timeframe5m)
	if err != nil {
		return fmt.Errorf("completed underlying candle: %w", err)
	}
	if len(candles) > 0 {
		e.UnderlyingCandle = &candles[0]
	}
	for _, position := range e.Positions {
		if position.Status != models.TradeOpen {
			continue
		}
		candles, err := e.repo.GetOptionCandles(e.ctx, position.ContractID, start, start, models.Timeframe5m)
		if err != nil {
			return fmt.Errorf("completed option candle for contract %d: %w", position.ContractID, err)
		}
		if len(candles) == 0 {
			continue
		}
		position.Candle = &candles[0]
	}
	return nil
}

func (e *Execution) fillOrders() error {
	for _, order := range e.Orders {
		if order.Status == models.OrderPending {
			if err := e.fillOrder(order); err != nil {
				return fmt.Errorf("order %s: %w", order.ID, err)
			}
		}
	}
	return nil
}

// fillOrder leaves orders pending when required candles are unavailable so a
// subsequent advance can fill them. Selected contracts are retained while waiting.
func (e *Execution) fillOrder(order *Order) error {
	if order.Contract == nil {
		contract, err := order.Selector.Select(e.ctx, e.underlyingID, e.UnderlyingCandle, e.expiry, e.repo)
		if err != nil || contract == nil {
			// Retry selection on a later advance while keeping the order pending.
			return nil
		}
		order.Contract = contract
	}

	var position *Position
	for _, candidate := range e.Positions {
		if candidate.ContractID == order.Contract.ID && candidate.Status == models.TradeOpen {
			position = candidate
			break
		}
	}
	var fill models.Candle
	if position != nil {
		closing := position.Side == models.Short && order.Side == "BUY" || position.Side == models.Long && order.Side == "SELL"
		if !closing {
			return fmt.Errorf("increasing an existing position is not supported")
		}
		if position.Quantity != order.Quantity {
			return fmt.Errorf("only full exits are supported: quantity must be %d", position.Quantity)
		}
		if position.Candle == nil {
			return nil
		}
		fill = *position.Candle
		position.ExitTime, position.ExitPrice = fill.Timestamp, fill.Open
		position.PnL = (position.EntryPrice - position.ExitPrice) * models.Price(position.Quantity)
		if position.Side == models.Long {
			position.PnL = -position.PnL
		}
		position.Status = models.TradeClosed
	} else {
		start := e.Time.Add(-candleInterval)
		// Query only the current completed window [start, e.Time). The repository
		// uses an inclusive end; PostgreSQL timestamps have microsecond precision.
		candles, err := e.repo.GetOptionCandles(e.ctx, order.Contract.ID, start, e.Time.Add(-time.Microsecond), models.Timeframe5m)
		if err != nil {
			return fmt.Errorf("order fill candle: %w", err)
		}
		if len(candles) == 0 {
			return nil
		}
		fill = candles[0]
		side := models.Long
		if order.Side == "SELL" {
			side = models.Short
		}
		position = &Position{Contract: order.Contract, Trade: models.Trade{ContractID: order.Contract.ID, Side: side, Quantity: order.Quantity, EntryTime: fill.Timestamp, EntryPrice: fill.Open, Status: models.TradeOpen}}
		position.Candle = &fill
		e.Positions = append(e.Positions, position)
	}
	order.FilledAt, order.FillPrice, order.Status = fill.Timestamp, fill.Open, models.OrderFilled
	return nil
}
