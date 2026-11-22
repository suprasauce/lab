package repository

import (
	"context"
	"fmt"

	"optionsbacktest/models"
)

// GetIndicatorCandles loads all stored 5-minute candles once per symbol.
// The returned slice is shared and must be treated as read-only. Indicators must
// exclude candles that have not completed by their simulation timestamp.
func (r *Repository) GetIndicatorCandles(ctx context.Context, tradingSymbol string) ([]models.Candle, error) {
	r.candlesMu.RLock()
	candles, ok := r.candles[tradingSymbol]
	r.candlesMu.RUnlock()
	if ok {
		return candles, nil
	}

	// Serialize loads so concurrent misses cannot populate the same key twice.
	r.candlesMu.Lock()
	defer r.candlesMu.Unlock()
	if candles, ok := r.candles[tradingSymbol]; ok {
		return candles, nil
	}
	r.logger.DebugContext(ctx, "loading indicator candles", "tradingsymbol", tradingSymbol)
	rows, err := r.db.QueryContext(ctx, queryGetIndicatorCandles, tradingSymbol)
	if err != nil {
		return nil, fmt.Errorf("query indicator candles for %q: %w", tradingSymbol, err)
	}
	defer rows.Close()

	candles = make([]models.Candle, 0)
	for rows.Next() {
		var candle models.Candle
		if err := rows.Scan(&candle.Timestamp, &candle.Timeframe, &candle.Open, &candle.High, &candle.Low, &candle.Close, &candle.Volume); err != nil {
			return nil, fmt.Errorf("scan indicator candle for %q: %w", tradingSymbol, err)
		}
		candles = append(candles, candle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indicator candles for %q: %w", tradingSymbol, err)
	}
	r.candles[tradingSymbol] = candles
	return candles, nil
}
