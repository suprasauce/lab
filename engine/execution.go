package engine

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
	"optionsbacktest/selector"
)

// Leg combines existing execution history with the current market-data snapshot.
type Leg struct {
	models.LegExecution
	Contract *models.Option
	Candle   *models.Candle
}

type order struct {
	close bool
	after time.Time
}

// Execution is a single expiry's runtime. It contains no scripting dependencies.
type Execution struct {
	UnderlyingCandle models.Candle
	DTE              int
	Legs             []*Leg
	ctx              context.Context
	repo             *repository.MarketDataRepository
	underlyingID     int64
	expiry           time.Time
	orders           map[string]order
}

func newExecution(ctx context.Context, repo *repository.MarketDataRepository, underlyingID int64, expiry time.Time) *Execution {
	return &Execution{ctx: ctx, repo: repo, underlyingID: underlyingID, expiry: expiry, orders: make(map[string]order)}
}

func (e *Execution) Leg(name string) (*Leg, error) {
	for _, leg := range e.Legs {
		if leg.Leg.ID == name {
			return leg, nil
		}
	}
	return nil, fmt.Errorf("unknown leg %q", name)
}

// SellCall resolves a strike from the observed candle and queues a short entry.
// A queued entry is still PENDING until filled at a subsequent candle's open.
func (e *Execution) SellCall(name string, quantity int64, offset int) error {
	if name == "" || quantity <= 0 {
		return fmt.Errorf("leg name must be nonempty and quantity must be positive")
	}
	for _, leg := range e.Legs {
		if leg.Leg.ID == name {
			return nil
		}
	}
	strike, err := selector.NewOffsetSelector(offset).SelectStrike(e.UnderlyingCandle)
	if err != nil {
		return err
	}
	contract, err := e.repo.GetOptionContract(e.ctx, e.underlyingID, e.expiry, strike, models.Call)
	if err != nil {
		return fmt.Errorf("select call for leg %s: %w", name, err)
	}
	leg := &Leg{
		LegExecution: models.LegExecution{
			Leg:    &models.OptionLeg{ID: name, Quantity: quantity, PositionSide: models.Short, OptionType: models.Call},
			Status: models.LegPending,
		},
		Contract: &contract,
	}
	e.Legs = append(e.Legs, leg)
	e.orders[name] = order{after: e.UnderlyingCandle.Timestamp.Add(5 * time.Minute)}
	return nil
}

func (e *Execution) Close(name string) error {
	leg, err := e.Leg(name)
	if err != nil {
		return err
	}
	if leg.Status != models.LegOpen {
		return nil
	}
	if _, exists := e.orders[name]; !exists {
		e.orders[name] = order{close: true, after: e.UnderlyingCandle.Timestamp.Add(5 * time.Minute)}
	}
	return nil
}

func (e *Execution) advance(candle models.Candle) error {
	e.UnderlyingCandle = candle
	day := calendarDate(candle.Timestamp.In(marketLocation))
	e.DTE = int(calendarDate(e.expiry).Sub(day) / (24 * time.Hour))
	for _, leg := range e.Legs {
		leg.Candle = nil
		pending, queued := e.orders[leg.Leg.ID]
		fill := queued && !candle.Timestamp.Before(pending.after)
		if leg.Status != models.LegOpen && !fill {
			continue
		}
		// Missing data leaves orders pending until a later simulation candle.
		candles, err := e.repo.GetOptionCandles(e.ctx, leg.Contract.ID, candle.Timestamp, candle.Timestamp, models.Timeframe5m)
		if err != nil {
			return fmt.Errorf("option candle for leg %s: %w", leg.Leg.ID, err)
		}
		if len(candles) == 0 {
			continue
		}
		leg.Candle = &candles[0]
		if !fill {
			continue
		}
		if pending.close {
			trade := &leg.Trades[len(leg.Trades)-1]
			trade.ExitTime = leg.Candle.Timestamp
			trade.ExitPrice = leg.Candle.Open
			trade.PnL = (trade.EntryPrice - trade.ExitPrice) * models.Price(trade.Quantity)
			trade.Status = models.TradeClosed
			leg.Status = models.LegClosed
		} else {
			leg.Trades = append(leg.Trades, models.Trade{
				ContractID: leg.Contract.ID, Side: models.Short, Quantity: leg.Leg.Quantity,
				EntryTime: leg.Candle.Timestamp, EntryPrice: leg.Candle.Open, Status: models.TradeOpen,
			})
			leg.Status = models.LegOpen
		}
		delete(e.orders, leg.Leg.ID)
	}
	return nil
}
