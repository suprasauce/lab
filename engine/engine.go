package engine

import (
	"context"
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

// Engine will evaluate each strategy leg independently over historical candles.
type Engine struct {
	marketDataRepository *repository.MarketDataRepository
}

func NewEngine(marketDataRepository *repository.MarketDataRepository) *Engine {
	return &Engine{marketDataRepository: marketDataRepository}
}

// Backtest is the generic entry point for all strategy types.
func (e *Engine) Backtest(ctx context.Context, strategy *models.Strategy) error {
	if strategy == nil {
		return fmt.Errorf("strategy cannot be nil")
	}

	switch strategy.Type {
	case models.StrategyOptions:
		return e.backtestOptions(ctx, strategy)
	case models.StrategyEquity:
		return e.backtestEquity(ctx, strategy)
	default:
		return fmt.Errorf("unsupported strategy type: %s", strategy.Type)
	}
}

func (e *Engine) backtestOptions(ctx context.Context, strategy *models.Strategy) error {
	expiries, err := e.marketDataRepository.GetExpiries(
		ctx,
		strategy.UnderlyingID,
		strategy.StartDate,
		strategy.EndDate,
	)
	if err != nil {
		return fmt.Errorf("backtest options: fetch expiries for underlying %d: %w", strategy.UnderlyingID, err)
	}
	if len(expiries) == 0 {
		return fmt.Errorf("backtest options: no expiries found for underlying %d between %s and %s",
			strategy.UnderlyingID, strategy.StartDate.Format(time.DateOnly), strategy.EndDate.Format(time.DateOnly))
	}
	for _, expiry := range expiries {
		if err := e.backtestExpiry(ctx, strategy, expiry); err != nil {
			return fmt.Errorf("backtest options: expiry %s: %w", expiry.Format(time.DateOnly), err)
		}
	}
	return nil
}

func (e *Engine) backtestExpiry(ctx context.Context, strategy *models.Strategy, expiry time.Time) error {
	maxEntryDTE := 0
	for i := range strategy.Legs {
		maxEntryDTE = max(maxEntryDTE, strategy.Legs[i].EntryDTE)
	}

	expiryDate := time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 0, 0, 0, 0, marketLocation)
	simulationStartDate := expiryDate.AddDate(0, 0, -maxEntryDTE)
	executions := make([]models.LegExecution, len(strategy.Legs))
	for i := range strategy.Legs {
		executions[i] = models.LegExecution{
			Leg:    &strategy.Legs[i],
			Status: models.LegPending,
		}
	}

	for day := simulationStartDate; !day.After(expiryDate); day = day.AddDate(0, 0, 1) {
		candles, err := e.marketDataRepository.GetUnderlyingCandles(
			ctx, strategy.UnderlyingID, day, day.AddDate(0, 0, 1), models.Timeframe5m,
		)
		if err != nil {
			return fmt.Errorf("backtest expiry %s: underlying candles for %s: %w", expiry.Format(time.DateOnly), day.Format(time.DateOnly), err)
		}
		for range candles {
			for i := range executions {
				execution := &executions[i]

				if execution.Status == models.LegPending {
				}

				if execution.Status == models.LegOpen {
				}

				if execution.Status == models.LegClosed {
				}
			}
		}
	}
	return nil
}

func (e *Engine) backtestEquity(ctx context.Context, strategy *models.Strategy) error {
	// TODO: Execute equity strategies over a continuous date range without expiry cycles.
	return errors.New("backtestEquity is not implemented")
}

// Run accepts candles already obtained for the strategy's configured time range.
// It will record trades on the supplied strategy. It is not implemented yet.
func (e *Engine) Run(strategy *models.Strategy, candles []models.Candle) error {
	// TODO: Iterate over candles chronologically within StartDate and EndDate.
	// TODO: At each timestamp, evaluate each leg independently:
	//   - Not entered: check entry criteria, select the instrument, and record
	//     one full entry trade for the configured quantity when triggered.
	//   - Open: check exit criteria and record one full exit trade when triggered.
	//   - Closed: ignore the leg; do not allow re-entry.
	// TODO: Derive lifecycle state from trades without adding redundant model state.
	return errors.New("backtesting engine is not implemented")
}
