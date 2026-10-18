package engine

import (
	"errors"

	"optionsbacktest/models"
)

// Engine will evaluate each strategy leg independently over historical candles.
type Engine struct{}

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
