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

// Execution contains trades and position history for one simulation.
type Execution struct {
	ID            string
	Time          time.Time
	tradeSequence int
	instruments   map[string]*instrument
	Positions     []*models.Position
	ChartService  *chart.ChartService
	ctx           context.Context
	repo          *repository.Repository
}

func newExecution(ctx context.Context, repo *repository.Repository, simulationID string) *Execution {
	return &Execution{ID: simulationID, ctx: ctx, repo: repo, ChartService: chart.NewChartService(repo)}
}

// PlaceTrade executes immediately at the script-supplied price in paise.
func (e *Execution) PlaceTrade(tradingSymbol string, quantity int64, price models.Price) (string, error) {
	trade := models.Trade{SimulationID: e.ID, TradingSymbol: tradingSymbol, Quantity: quantity, Price: price, Timestamp: e.Time}
	if err := trade.Validate(); err != nil {
		return "", err
	}
	instrument, err := e.resolveInstrument(tradingSymbol)
	if err != nil {
		return "", fmt.Errorf("resolve instrument %q: %w", tradingSymbol, err)
	}
	var position *models.Position
	for _, candidate := range e.Positions {
		if candidate.TradingSymbol == tradingSymbol {
			position = candidate
			break
		}
	}
	if position != nil && position.Quantity != 0 {
		closing := position.Quantity < 0 && quantity > 0 || position.Quantity > 0 && quantity < 0
		if !closing {
			return "", fmt.Errorf("increasing an existing position is not supported")
		}
		if position.Quantity != -quantity {
			return "", fmt.Errorf("only full exits are supported: quantity must be %d", -position.Quantity)
		}
		position.RealizedPnL += (price - position.AveragePrice) * models.Price(position.Quantity) * instrument.multiplier()
		position.Quantity, position.UnrealizedPnL, position.Candle = 0, 0, nil
	} else {
		candle, err := e.currentCandle(instrument)
		if err != nil {
			return "", err
		}
		if position == nil {
			position = &models.Position{SimulationID: e.ID, TradingSymbol: tradingSymbol}
			e.Positions = append(e.Positions, position)
		}
		position.Quantity, position.AveragePrice, position.Candle = quantity, price, candle
		position.UnrealizedPnL = 0
	}
	e.tradeSequence++
	trade.ID = fmt.Sprintf("%s/%d", e.ID, e.tradeSequence)
	position.Trades = append(position.Trades, trade)
	e.recordPnL(position)
	return trade.ID, nil
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

// advance marks the candle that just finished, then exposes current-minute OHLC.
func (e *Execution) advance(at time.Time) error {
	e.Time = at
	for _, position := range e.Positions {
		completed := position.Candle
		position.Candle = nil
		if position.Quantity == 0 {
			position.UnrealizedPnL = 0
		} else {
			candle, err := e.GetCandle(position.TradingSymbol)
			if err != nil {
				return err
			}
			position.Candle = candle
			if completed == nil || !completed.Timestamp.Add(candleInterval).Equal(at) {
				continue
			}
			instrument, err := e.resolveInstrument(position.TradingSymbol)
			if err != nil {
				return err
			}
			position.UnrealizedPnL = (completed.Close - position.AveragePrice) * models.Price(position.Quantity) * instrument.multiplier()
		}
		e.recordPnL(position)
	}
	return nil
}

// GetCandle exposes full OHLC for the current simulation minute, or nil if missing.
// Scripts are responsible for the chronological use of these values.
func (e *Execution) GetCandle(symbol string) (*models.Candle, error) {
	instrument, err := e.resolveInstrument(symbol)
	if err != nil {
		return nil, fmt.Errorf("resolve instrument %q: %w", symbol, err)
	}
	return e.currentCandle(instrument)
}

func (e *Execution) currentCandle(instrument *instrument) (*models.Candle, error) {
	start := e.Time.Truncate(time.Minute)
	candles, err := e.instrumentCandles(instrument, start, start.Add(time.Microsecond))
	if err != nil {
		return nil, fmt.Errorf("current candle for %q: %w", instrument.TradingSymbol, err)
	}
	if len(candles) == 0 {
		return nil, nil
	}
	return &candles[0], nil
}

func (e *Execution) recordPnL(position *models.Position) {
	point := models.PnLPoint{Timestamp: e.Time, PnL: position.RealizedPnL + position.UnrealizedPnL}
	if n := len(position.TotalPnL); n > 0 && position.TotalPnL[n-1].Timestamp.Equal(e.Time) {
		position.TotalPnL[n-1] = point
	} else {
		position.TotalPnL = append(position.TotalPnL, point)
	}
}

// Resolve and reuse instrument metadata independently of trade history.
func (e *Execution) resolveInstrument(tradingSymbol string) (*instrument, error) {
	if cached := e.instruments[tradingSymbol]; cached != nil {
		return cached, nil
	}
	asset := &instrument{TradingSymbol: tradingSymbol}
	underlying, err := e.repo.GetUnderlying(e.ctx, tradingSymbol)
	if err == nil {
		asset.Underlying = &underlying
	} else {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		contract, err := e.repo.GetOptionContractByTradingSymbol(e.ctx, tradingSymbol)
		if err != nil {
			return nil, err
		}
		asset.Contract = &contract
	}
	if e.instruments == nil {
		e.instruments = make(map[string]*instrument)
	}
	e.instruments[tradingSymbol] = asset
	return asset, nil
}

// instrumentCandles uses an exclusive end for both instrument kinds.
func (e *Execution) instrumentCandles(instrument *instrument, start, end time.Time) ([]models.Candle, error) {
	if instrument.Underlying != nil {
		return e.repo.GetUnderlyingCandles(e.ctx, instrument.Underlying.ID, start, end, models.Timeframe1m)
	}
	return e.repo.GetOptionCandles(e.ctx, instrument.Contract.ID, start, end.Add(-time.Microsecond), models.Timeframe1m)
}
