package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

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
		strategy.StartTime,
		strategy.EndTime,
	)
	if err != nil {
		return fmt.Errorf("backtest options: fetch expiries for underlying %d: %w", strategy.UnderlyingID, err)
	}
	if len(expiries) == 0 {
		return fmt.Errorf("backtest options: no expiries found for underlying %d between %s and %s",
			strategy.UnderlyingID, strategy.StartTime.Format(time.DateOnly), strategy.EndTime.Format(time.DateOnly))
	}
	for _, expiry := range expiries {
		if err := e.backtestExpiry(ctx, strategy, expiry); err != nil {
			return fmt.Errorf("backtest options: expiry %s: %w", expiry.Format(time.DateOnly), err)
		}
	}
	return nil
}

func (e *Engine) backtestExpiry(ctx context.Context, strategy *models.Strategy, expiry time.Time) error {
	// TODO: Invoke backtestLeg for each configured leg using the shared expiry.
	return errors.New("backtestExpiry is not implemented")
}

func (e *Engine) backtestLeg(ctx context.Context, leg *models.Leg, expiry time.Time) error {
	// TODO: Determine entry and exit timestamps, resolve the option contract,
	// fetch historical option candles, and simulate the leg.
	return errors.New("backtestLeg is not implemented")
}

func (e *Engine) backtestEquity(ctx context.Context, strategy *models.Strategy) error {
	// TODO: Execute equity strategies over a continuous date range without expiry cycles.
	return errors.New("backtestEquity is not implemented")
}

// Run accepts candles already obtained for the strategy's configured time range.
// It will record trades on the supplied strategy. It is not implemented yet.
func (e *Engine) Run(strategy *models.Strategy, candles []models.Candle) error {
	// TODO: Iterate over candles chronologically within StartTime and EndTime.
	// TODO: At each timestamp, evaluate each leg independently:
	//   - Not entered: check entry criteria, select the instrument, and record
	//     one full entry trade for the configured quantity when triggered.
	//   - Open: check exit criteria and record one full exit trade when triggered.
	//   - Closed: ignore the leg; do not allow re-entry.
	// TODO: Derive lifecycle state from trades without adding redundant model state.
	return errors.New("backtesting engine is not implemented")
}
