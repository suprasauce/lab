package engine

import (
	"context"
	"database/sql"
	"errors"
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
const candleInterval = 5 * time.Minute
const timeLayout = "15:04"

// The fixed HH:mm literal is valid and parsed once with the default date.
var sessionStartTime, _ = time.Parse(timeLayout, "09:15")

type Engine struct {
	marketDataRepository *repository.MarketDataRepository
}

func NewEngine(repo *repository.MarketDataRepository) *Engine {
	return &Engine{marketDataRepository: repo}
}

// Backtest resolves the underlying and selects expiry cycles within the inclusive date range. Each cycle
// starts 45 calendar days before expiry, even if that precedes startDate.
// onCandle observes only completed candles. Orders can fill at the current open.
// Returned executions retain trade histories, including positions left open.
func (e *Engine) Backtest(ctx context.Context, symbol string, startDate, endDate time.Time, onCandle func(*Execution) error) (*BacktestResult, error) {
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
	results := &BacktestResult{Underlying: underlying.Symbol, Executions: make([][]*Position, 0, len(expiries))}
	for _, expiry := range expiries {
		execution := newExecution(ctx, e.marketDataRepository, underlyingID, expiry)
		err := e.backtestExpiry(ctx, underlyingID, expiry, calendarDate(expiry).AddDate(0, 0, -SimulationDTE), func(at time.Time) error {
			if err := execution.advance(at); err != nil {
				return err
			}
			if at.Format(timeLayout) == "15:30" {
				return nil // Finalize the session without allowing new strategy orders.
			}
			return onCandle(execution)
		})
		results.Executions = append(results.Executions, execution.Positions)
		if err == nil {
			for _, order := range execution.Orders {
				if order.Status == models.OrderPending {
					err = fmt.Errorf("unfilled order %s remains at expiry", order.ID)
					break
				}
			}
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

func (e *Engine) backtestExpiry(ctx context.Context, underlyingID int64, expiry, startDate time.Time, onTime func(time.Time) error) error {
	expiryDate := calendarDate(expiry)
	for day := calendarDate(startDate); !day.After(expiryDate); day = day.AddDate(0, 0, 1) {
		start := day.Add(9*time.Hour + 15*time.Minute)
		end := day.Add(15*time.Hour + 30*time.Minute)
		// Probe for session data without loading the day's candle slice. This
		// candle is used only to establish that a session exists, never exposed.
		first, err := e.marketDataRepository.GetUnderlyingCandle(ctx, underlyingID, start, models.Timeframe5m)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("backtest expiry %s: underlying candles for %s: %w", expiry.Format(time.DateOnly), day.Format(time.DateOnly), err)
		}
		if !first.Timestamp.Before(end) {
			continue
		}
		for at := start; !at.After(end); at = at.Add(candleInterval) {
			if err := onTime(at); err != nil {
				return fmt.Errorf("simulation time %s: %w", at.Format(time.RFC3339), err)
			}
		}
	}
	return nil
}
