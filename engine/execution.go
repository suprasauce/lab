package engine

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

// Position reuses Trade's complete entry/exit lifecycle and price representation.
type Position struct {
	models.Trade
	Contract *models.Option
	Candle   *models.Candle
}

// Execution contains fresh orders and position history for one expiry.
type Execution struct {
	Time      time.Time
	Orders    []*Order
	Positions []*Position
	ctx       context.Context
	repo      *repository.Repository
}

func newExecution(ctx context.Context, repo *repository.Repository) *Execution {
	return &Execution{ctx: ctx, repo: repo}
}

// PlaceOrder queues an opaque trading symbol without resolving or executing it.
func (e *Execution) PlaceOrder(tradingSymbol string, quantity int64) (string, error) {
	order := &Order{Order: models.Order{TradingSymbol: tradingSymbol, Quantity: quantity}}
	if err := order.Validate(); err != nil {
		return "", err
	}
	order.GenerateID(len(e.Orders) + 1)
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
	for _, position := range e.Positions {
		position.Candle = nil
	}
	e.Time = at
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
		contract, err := e.repo.GetOptionContractByTradingSymbol(e.ctx, order.TradingSymbol)
		if err != nil {
			return fmt.Errorf("resolve option contract: %w", err)
		}
		order.Contract = &contract
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
		closing := position.Quantity < 0 && order.Quantity > 0 || position.Quantity > 0 && order.Quantity < 0
		if !closing {
			return fmt.Errorf("increasing an existing position is not supported")
		}
		if position.Quantity != -order.Quantity {
			return fmt.Errorf("only full exits are supported: quantity must be %d", -position.Quantity)
		}
		if position.Candle == nil {
			return nil
		}
		fill = *position.Candle
		position.ExitTime, position.ExitPrice = fill.Timestamp, fill.Open
		position.PnL = (position.ExitPrice - position.EntryPrice) * models.Price(position.Quantity)
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
		position = &Position{Contract: order.Contract, Trade: models.Trade{ContractID: order.Contract.ID, Quantity: order.Quantity, EntryTime: fill.Timestamp, EntryPrice: fill.Open, Status: models.TradeOpen}}
		position.Candle = &fill
		e.Positions = append(e.Positions, position)
	}
	order.FilledAt, order.FillPrice, order.Status = fill.Timestamp, fill.Open, models.OrderFilled
	return nil
}
