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

const tradingCalendarUnderlyingID int64 = 1
const candleInterval = time.Minute
const timeLayout = "15:04"

// The fixed HH:mm literal is valid and parsed once with the default date.
var sessionStartTime, _ = time.Parse(timeLayout, "09:15")

type Engine struct {
	marketDataRepository *repository.Repository
}

func NewEngine(repo *repository.Repository) *Engine {
	return &Engine{marketDataRepository: repo}
}

// Repository exposes the existing data dependency for scripting service wiring.
func (e *Engine) Repository() *repository.Repository {
	return e.marketDataRepository
}

// GetExpiries resolves the underlying and discovers available expiry dates.
func (e *Engine) GetExpiries(ctx context.Context, symbol string, start, end time.Time) ([]time.Time, error) {
	underlying, err := e.marketDataRepository.GetUnderlying(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("resolve underlying: %w", err)
	}
	return e.marketDataRepository.GetExpiries(ctx, underlying.ID, start, end)
}

// Simulate executes an inclusive calendar-date range with fresh runtime state.
func (e *Engine) Simulate(ctx context.Context, simulationID string, startDate, endDate time.Time, onCandle func(*Execution) error) (*BacktestResult, error) {
	if onCandle == nil {
		return nil, fmt.Errorf("on_candle callback is required")
	}
	startDate, endDate = calendarDate(startDate), calendarDate(endDate)
	if startDate.After(endDate) {
		return nil, fmt.Errorf("start_date must not be after end_date")
	}
	execution := newExecution(ctx, e.marketDataRepository, simulationID)
	err := e.simulateRange(ctx, startDate, endDate, func(at time.Time) error {
		if err := execution.advance(at); err != nil {
			return err
		}
		if at.Format(timeLayout) == "15:30" {
			return nil
		}
		return onCandle(execution)
	})
	result := &BacktestResult{Executions: []*Execution{execution}}
	if err != nil {
		return result, fmt.Errorf("simulation %s through %s: %w", startDate.Format(time.DateOnly), endDate.Format(time.DateOnly), err)
	}
	return result, nil
}

func calendarDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, marketLocation)
}

func (e *Engine) simulateRange(ctx context.Context, startDate, endDate time.Time, onTime func(time.Time) error) error {
	for day := calendarDate(startDate); !day.After(calendarDate(endDate)); day = day.AddDate(0, 0, 1) {
		start := day.Add(9*time.Hour + 15*time.Minute)
		end := day.Add(15*time.Hour + 30*time.Minute)
		// Probe for session data without loading the day's candle slice. This
		// candle is used only to establish that a session exists, never exposed.
		first, err := e.marketDataRepository.GetUnderlyingCandle(ctx, tradingCalendarUnderlyingID, start, models.Timeframe1m)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("underlying candles for %s: %w", day.Format(time.DateOnly), err)
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
