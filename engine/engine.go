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

// Engine iterates historical candles for options backtests.
type Engine struct {
	marketDataRepository *repository.MarketDataRepository
}

func NewEngine(marketDataRepository *repository.MarketDataRepository) *Engine {
	return &Engine{marketDataRepository: marketDataRepository}
}

// Backtest runs an options strategy.
func (e *Engine) Backtest(ctx context.Context, strategy *models.Strategy, underlyingID int64, startDate, endDate time.Time) error {
	if strategy == nil {
		return fmt.Errorf("strategy cannot be nil")
	}

	return e.backtestOptions(ctx, underlyingID, startDate, endDate)
}

func (e *Engine) backtestOptions(ctx context.Context, underlyingID int64, startDate, endDate time.Time) error {
	expiries, err := e.marketDataRepository.GetExpiries(
		ctx,
		underlyingID,
		startDate,
		endDate,
	)
	if err != nil {
		return fmt.Errorf("backtest options: fetch expiries for underlying %d: %w", underlyingID, err)
	}
	if len(expiries) == 0 {
		return fmt.Errorf("backtest options: no expiries found for underlying %d between %s and %s",
			underlyingID, startDate.Format(time.DateOnly), endDate.Format(time.DateOnly))
	}
	for _, expiry := range expiries {
		if err := e.backtestExpiry(ctx, underlyingID, expiry, startDate); err != nil {
			return fmt.Errorf("backtest options: expiry %s: %w", expiry.Format(time.DateOnly), err)
		}
	}
	return nil
}

func (e *Engine) backtestExpiry(ctx context.Context, underlyingID int64, expiry, startDate time.Time) error {
	expiryDate := time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 0, 0, 0, 0, marketLocation)
	simulationStartDate := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, marketLocation)

	for day := simulationStartDate; !day.After(expiryDate); day = day.AddDate(0, 0, 1) {
		candles, err := e.marketDataRepository.GetUnderlyingCandles(
			ctx, underlyingID, day, day.AddDate(0, 0, 1), models.Timeframe5m,
		)
		if err != nil {
			return fmt.Errorf("backtest expiry %s: underlying candles for %s: %w", expiry.Format(time.DateOnly), day.Format(time.DateOnly), err)
		}
		for range candles {
		}
	}
	return nil
}
