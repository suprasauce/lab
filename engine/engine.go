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
	executions := make([]models.LegExecution, len(strategy.Legs))
	for i := range strategy.Legs {
		executions[i] = models.LegExecution{
			Leg: strategy.Legs[i],
			State: models.LegState{
				Status: models.LegPending,
			},
		}
	}
	for i := range executions {
		leg, ok := executions[i].Leg.(*models.OptionLeg)
		if !ok || leg == nil {
			return fmt.Errorf("backtest expiry %s: leg %d must be a non-nil *models.OptionLeg (got %T)", expiry.Format(time.DateOnly), i, executions[i].Leg)
		}
		if err := e.backtestOptionLeg(ctx, leg, &executions[i].State, expiry, strategy.UnderlyingID); err != nil {
			return fmt.Errorf("backtest expiry %s: leg %d: %w", expiry.Format(time.DateOnly), i, err)
		}
	}
	return nil
}

func (e *Engine) backtestOptionLeg(ctx context.Context, leg *models.OptionLeg, state *models.LegState, expiry time.Time, underlyingID int64) error {
	switch state.Status {
	case models.LegPending:
		entryDate := expiry.AddDate(0, 0, -leg.EntryDTE)
		state.EntryTargetTime = time.Date(
			entryDate.Year(), entryDate.Month(), entryDate.Day(),
			leg.EntryHour, leg.EntryMinute, 0, 0, marketLocation,
		)

		contract, err := leg.InstrumentSelector.Select(ctx, models.SelectionInput{
			UnderlyingID: underlyingID,
			Expiry:       expiry,
			EntryTime:    state.EntryTargetTime,
			OptionType:   leg.OptionType,
			Timeframe:    models.Timeframe1m,
		})
		if err != nil {
			return fmt.Errorf("select option contract: %w", err)
		}
		state.SelectedContract = &contract
		state.SelectedStrike = contract.StrikePrice

	case models.LegOpen:
		// TODO: Exit logic.
	case models.LegClosed:
		return nil
	default:
		return fmt.Errorf("invalid leg status: %s", state.Status)
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
	// TODO: Iterate over candles chronologically within StartTime and EndTime.
	// TODO: At each timestamp, evaluate each leg independently:
	//   - Not entered: check entry criteria, select the instrument, and record
	//     one full entry trade for the configured quantity when triggered.
	//   - Open: check exit criteria and record one full exit trade when triggered.
	//   - Closed: ignore the leg; do not allow re-entry.
	// TODO: Derive lifecycle state from trades without adding redundant model state.
	return errors.New("backtesting engine is not implemented")
}
