package engine

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
	"optionsbacktest/selector"
)

// Leg combines execution history with the latest completed option candle.
type Leg struct {
	models.LegExecution
	Contract *models.Option
	Candle   *models.Candle
}

type order struct {
	close    bool
	offset   int
	PlacedAt time.Time
	// Prevent a newly selected contract from filling at an earlier historical open.
	selectedAt time.Time
}

// Execution is one expiry's runtime, independent of the scripting language.
type Execution struct {
	Time             time.Time
	UnderlyingCandle *models.Candle
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

// Open queues a market entry for the next advance(). PlacedAt preserves
// the earliest eligible execution timestamp independently of processing time.
func (e *Execution) Open(legID string, optionType string, side models.PositionSide, quantity int64, offset int) error {
	if legID == "" || quantity <= 0 {
		return fmt.Errorf("leg name must be nonempty and quantity must be positive")
	}
	var contractType models.OptionType
	switch optionType {
	case "CE":
		contractType = models.Call
	case "PE":
		contractType = models.Put
	default:
		return fmt.Errorf("invalid option_type %q: expected CE or PE", optionType)
	}
	if side != models.Long && side != models.Short {
		return fmt.Errorf("invalid side: %s", side)
	}
	for _, leg := range e.Legs {
		if leg.Leg.ID == legID {
			return nil
		}
	}
	e.Legs = append(e.Legs, &Leg{LegExecution: models.LegExecution{
		Leg:    &models.OptionLeg{ID: legID, Quantity: quantity, PositionSide: side, OptionType: contractType},
		Status: models.LegPending,
	}})
	e.orders[legID] = order{PlacedAt: e.Time, offset: offset}
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
		e.orders[name] = order{close: true, PlacedAt: e.Time}
	}
	return nil
}

// advance publishes completed bars only, then fills previously queued orders
// before the next strategy evaluation. Missing required completed bars are errors.
func (e *Execution) advance(at time.Time) error {
	day := calendarDate(at)
	e.UnderlyingCandle = nil
	for _, leg := range e.Legs {
		leg.Candle = nil
	}
	e.Time = at
	e.DTE = int(calendarDate(e.expiry).Sub(day) / (24 * time.Hour))
	if err := e.loadCompletedCandles(); err != nil {
		return err
	}
	return e.fillOrders()
}

func (e *Execution) loadCompletedCandles() error {
	start := e.Time.Add(-candleInterval)
	// loads the candles having close at or before ctx.time, before we callt the script for this ctx.time with updated candles
	if startTime, _ := time.Parse(timeLayout, start.Format(timeLayout)); startTime.Before(sessionStartTime) {
		return nil
	}
	// A microsecond-wide range is an exact PostgreSQL timestamp lookup.
	candles, err := e.repo.GetUnderlyingCandles(e.ctx, e.underlyingID, start, start.Add(time.Microsecond), models.Timeframe5m)
	if err != nil {
		return fmt.Errorf("completed underlying candle: %w", err)
	}
	if len(candles) == 0 {
		return fmt.Errorf("missing completed underlying candle for underlying %d at %s: %w", e.underlyingID, start.Format(time.RFC3339), sql.ErrNoRows)
	}
	e.UnderlyingCandle = &candles[0]
	for _, leg := range e.Legs {
		if leg.Status != models.LegOpen {
			continue
		}
		candles, err := e.repo.GetOptionCandles(e.ctx, leg.Contract.ID, start, start, models.Timeframe5m)
		if err != nil {
			return fmt.Errorf("completed option candle for leg %s: %w", leg.Leg.ID, err)
		}
		if len(candles) == 0 {
			return fmt.Errorf("missing completed option candle for leg %s contract %d at %s: %w", leg.Leg.ID, leg.Contract.ID, start.Format(time.RFC3339), sql.ErrNoRows)
		}
		leg.Candle = &candles[0]
	}
	return nil
}

// fillOrders never publishes the fill candle's unfinished OHLC to the script.
// A fill's actual timestamp is stored in Trade.EntryTime or Trade.ExitTime,
// separately from the queued order's PlacedAt.
func (e *Execution) fillOrders() error {
	for _, leg := range e.Legs {
		if pending, queued := e.orders[leg.Leg.ID]; queued {
			if err := e.fillOrder(leg, pending); err != nil {
				return err
			}
		}
	}
	return nil
}

// fillOrder leaves the order pending when a required candle is unavailable,
// allowing it to fill using a subsequent candle on a later advance.
func (e *Execution) fillOrder(leg *Leg, pending order) error {
	if !pending.close { // Open a position.
		if leg.Contract == nil {
			underlying := e.UnderlyingCandle
			if underlying == nil {
				return nil
			}
			contract, err := e.selectOptionContract(leg.Leg.ID, pending.offset, *underlying, leg.Leg.OptionType)
			if err != nil {
				return err
			}
			leg.Contract = &contract
			pending.selectedAt = underlying.Timestamp
			e.orders[leg.Leg.ID] = pending
		}
		candles, err := e.repo.GetOptionCandles(e.ctx, leg.Contract.ID, pending.selectedAt, e.Time, models.Timeframe5m)
		if err != nil {
			return fmt.Errorf("order fill candle for leg %s: %w", leg.Leg.ID, err)
		}
		if len(candles) == 0 {
			return nil
		}
		fill := candles[0]
		leg.Trades = append(leg.Trades, models.Trade{
			ContractID: leg.Contract.ID, Side: leg.Leg.PositionSide, Quantity: leg.Leg.Quantity,
			EntryTime: fill.Timestamp, EntryPrice: fill.Open, Status: models.TradeOpen,
		})
		leg.Status = models.LegOpen
		if fill.Timestamp.Equal(e.Time.Add(-candleInterval)) {
			leg.Candle = &fill // This fill candle has completed by processing time.
		}
	} else { // Close a position.
		if leg.Candle == nil {
			return nil
		}
		trade := &leg.Trades[len(leg.Trades)-1]
		trade.ExitTime, trade.ExitPrice = leg.Candle.Timestamp, leg.Candle.Open
		trade.PnL = (trade.EntryPrice - trade.ExitPrice) * models.Price(trade.Quantity)
		if trade.Side == models.Long {
			trade.PnL = -trade.PnL
		}
		trade.Status, leg.Status = models.TradeClosed, models.LegClosed
	}
	delete(e.orders, leg.Leg.ID)
	return nil
}
func (e *Execution) selectOptionContract(legID string, offset int, underlying models.Candle, optionType models.OptionType) (models.Option, error) {
	strike, err := selector.NewOffsetSelector(offset).SelectStrike(models.Candle{Open: underlying.Open})
	if err != nil {
		return models.Option{}, err
	}
	contract, err := e.repo.GetOptionContract(e.ctx, e.underlyingID, e.expiry, strike, optionType)
	if err != nil {
		return models.Option{}, fmt.Errorf("select option contract for leg %s: %w", legID, err)
	}
	return contract, nil
}
