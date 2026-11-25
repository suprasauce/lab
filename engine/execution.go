package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"optionsbacktest/chart"
	"optionsbacktest/models"
	"optionsbacktest/repository"
)

// Execution contains fresh orders and position history for one expiry.
type Execution struct {
	ID           string
	Time         time.Time
	Orders       []*Order
	Positions    []*models.Position
	ChartService *chart.ChartService
	ctx          context.Context
	repo         *repository.Repository
}

func newExecution(ctx context.Context, repo *repository.Repository, simulationID string) *Execution {
	return &Execution{ID: simulationID, ctx: ctx, repo: repo, ChartService: chart.NewChartService(repo)}
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

// GetPositions returns the matching position, if any, whether open or closed.
func (e *Execution) GetPositions(tradingSymbol string) []*models.Position {
	positions := make([]*models.Position, 0)
	for _, position := range e.Positions {
		if position.TradingSymbol == tradingSymbol {
			positions = append(positions, position)
		}
	}
	return positions
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
	if err := e.fillOrders(); err != nil {
		return err
	}
	for _, position := range e.Positions {
		if position.Quantity == 0 {
			position.UnrealizedPnL = 0
		} else {
			if position.Candle == nil {
				continue
			}
			instrument, err := e.resolveInstrument(position.TradingSymbol)
			if err != nil {
				return fmt.Errorf("position P&L instrument: %w", err)
			}
			position.UnrealizedPnL = (position.Candle.Close - position.AveragePrice) * models.Price(position.Quantity) * instrument.multiplier()
		}
		position.TotalPnL = append(position.TotalPnL, models.PnLPoint{Timestamp: e.Time, PnL: position.RealizedPnL + position.UnrealizedPnL})
	}
	return nil
}

func (e *Execution) loadCompletedCandles() error {
	start := e.Time.Add(-candleInterval)
	if startTime, _ := time.Parse(timeLayout, start.Format(timeLayout)); startTime.Before(sessionStartTime) {
		return nil
	}
	for _, position := range e.Positions {
		if position.Quantity == 0 {
			continue
		}
		instrument, err := e.resolveInstrument(position.TradingSymbol)
		if err != nil {
			return fmt.Errorf("position instrument: %w", err)
		}
		candles, err := e.instrumentCandles(instrument, start, start.Add(time.Microsecond))
		if err != nil {
			return fmt.Errorf("completed candle for %q: %w", position.TradingSymbol, err)
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
	if order.Contract == nil && order.Underlying == nil {
		instrument, err := e.resolveInstrument(order.TradingSymbol)
		if err != nil {
			return fmt.Errorf("resolve instrument for trading symbol %q: %w", order.TradingSymbol, err)
		}
		order.Contract, order.Underlying = instrument.Contract, instrument.Underlying
	}

	var position *models.Position
	for _, candidate := range e.Positions {
		if candidate.TradingSymbol == order.TradingSymbol {
			position = candidate
			break
		}
	}
	var fill models.Candle
	if position != nil && position.Quantity != 0 {
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
		position.RealizedPnL += (fill.Open - position.AveragePrice) * models.Price(position.Quantity) * order.multiplier()
		position.Quantity = 0
		position.Candle = nil
	} else {
		start := e.Time.Add(-candleInterval)
		// Query only the current completed window [start, e.Time).
		candles, err := e.instrumentCandles(order, start, e.Time)
		if err != nil {
			return fmt.Errorf("order fill candle: %w", err)
		}
		if len(candles) == 0 {
			return nil
		}
		fill = candles[0]
		if position == nil {
			position = &models.Position{SimulationID: e.ID, TradingSymbol: order.TradingSymbol}
			e.Positions = append(e.Positions, position)
		}
		position.Quantity, position.AveragePrice = order.Quantity, fill.Open
		position.Candle = &fill
	}
	position.Trades = append(position.Trades, models.Trade{
		ID: fmt.Sprintf("%s/%s", e.ID, order.ID), SimulationID: e.ID,
		TradingSymbol: order.TradingSymbol, Timestamp: fill.Timestamp, Price: fill.Open, Quantity: order.Quantity,
	})
	order.FilledAt, order.FillPrice, order.Status = fill.Timestamp, fill.Open, models.OrderFilled
	return nil
}

// Reuse metadata already resolved for an order; positions only retain the symbol.
func (e *Execution) resolveInstrument(tradingSymbol string) (*Order, error) {
	for _, order := range e.Orders {
		if order.TradingSymbol == tradingSymbol && (order.Contract != nil || order.Underlying != nil) {
			return order, nil
		}
	}
	instrument := &Order{Order: models.Order{TradingSymbol: tradingSymbol}}
	underlying, err := e.repo.GetUnderlying(e.ctx, tradingSymbol)
	if err == nil {
		instrument.Underlying = &underlying
		return instrument, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	contract, err := e.repo.GetOptionContractByTradingSymbol(e.ctx, tradingSymbol)
	if err != nil {
		return nil, err
	}
	instrument.Contract = &contract
	return instrument, nil
}

// instrumentCandles uses an exclusive end for both instrument kinds.
func (e *Execution) instrumentCandles(instrument *Order, start, end time.Time) ([]models.Candle, error) {
	if instrument.Underlying != nil {
		return e.repo.GetUnderlyingCandles(e.ctx, instrument.Underlying.ID, start, end, models.Timeframe1m)
	}
	return e.repo.GetOptionCandles(e.ctx, instrument.Contract.ID, start, end.Add(-time.Microsecond), models.Timeframe1m)
}
