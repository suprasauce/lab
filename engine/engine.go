package engine

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

var marketLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		panic(fmt.Errorf("load market timezone: %w", err))
	}
	return location
}()

const SimulationDTE = 45

type Engine struct {
	marketDataRepository *repository.MarketDataRepository
}

func NewEngine(repo *repository.MarketDataRepository) *Engine {
	return &Engine{marketDataRepository: repo}
}

// Backtest resolves the underlying and selects expiry cycles within the inclusive date range. Each cycle
// starts 45 calendar days before expiry, even if that precedes startDate.
// onCandle observes completed 5-minute candles and queues orders for a later bar.
// Returned executions retain trade histories, including positions left open.
func (e *Engine) Backtest(ctx context.Context, symbol string, startDate, endDate time.Time, legs []models.OptionLeg, onCandle func(*Execution) error) ([][]models.LegExecution, error) {
	if onCandle == nil {
		return nil, fmt.Errorf("on_candle callback is required")
	}
	underlying, err := e.marketDataRepository.GetUnderlying(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("resolve underlying: %w", err)
	}
	underlyingID := underlying.ID
	expiries, err := e.marketDataRepository.GetExpiries(ctx, underlyingID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("fetch expiries for underlying %d: %w", underlyingID, err)
	}
	if len(expiries) == 0 {
		return nil, fmt.Errorf("no expiries found for underlying %d", underlyingID)
	}
	results := make([][]models.LegExecution, 0, len(expiries))
	for _, expiry := range expiries {
		execution := newExecution(ctx, e.marketDataRepository, underlyingID, expiry, legs)
		err := e.backtestExpiry(ctx, underlyingID, expiry, calendarDate(expiry).AddDate(0, 0, -SimulationDTE), func(candle models.Candle) error {
			if err := execution.advance(candle); err != nil {
				return err
			}
			return onCandle(execution)
		})
		results = append(results, execution.history())
		if err == nil && len(execution.orders) != 0 {
			err = fmt.Errorf("unfilled orders remain at expiry")
		}
		if err != nil {
			return results, fmt.Errorf("expiry %s: %w", expiry.Format(time.DateOnly), err)
		}
	}
	return results, nil
}

func calendarDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, marketLocation)
}

func (e *Engine) backtestExpiry(ctx context.Context, underlyingID int64, expiry, startDate time.Time, onCandle func(models.Candle) error) error {
	expiryDate := calendarDate(expiry)
	for day := calendarDate(startDate); !day.After(expiryDate); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return err
		}
		candles, err := e.marketDataRepository.GetUnderlyingCandles(ctx, underlyingID, day, day.AddDate(0, 0, 1), models.Timeframe5m)
		if err != nil {
			return fmt.Errorf("backtest expiry %s: underlying candles for %s: %w", expiry.Format(time.DateOnly), day.Format(time.DateOnly), err)
		}
		for _, candle := range candles {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := onCandle(candle); err != nil {
				return fmt.Errorf("candle %s: %w", candle.Timestamp.Format(time.RFC3339), err)
			}
		}
	}
	return nil
}
