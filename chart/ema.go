package chart

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
)

type emaMetadata struct {
	Value     float64
	NextIndex int
}

// EMA advances the indicator using only newly completed 5-minute candle closes.
// The first completed close seeds the EMA; values are returned in rupees.
func (s *ChartService) EMA(ctx context.Context, tradingSymbol string, period int, timeframe models.Timeframe, at time.Time) (*float64, error) {
	candles, err := s.repo.GetIndicatorCandles(ctx, tradingSymbol)
	if err != nil {
		return nil, fmt.Errorf("EMA candles: %w", err)
	}
	key := fmt.Sprintf("ema:%s:%d:%s", tradingSymbol, period, timeframe)
	metadata, ok := s.indicators[key].(*emaMetadata)
	if !ok {
		metadata = &emaMetadata{}
		s.indicators[key] = metadata
	}
	alpha := 2.0 / (float64(period) + 1)
	for metadata.NextIndex < len(candles) {
		candle := &candles[metadata.NextIndex]
		if candle.Timestamp.Add(5 * time.Minute).After(at) {
			break
		}
		close := float64(candle.Close) / 100
		if metadata.NextIndex == 0 {
			metadata.Value = close
		} else {
			metadata.Value = close*alpha + metadata.Value*(1-alpha)
		}
		metadata.NextIndex++
	}
	if metadata.NextIndex == 0 {
		return nil, nil
	}
	value := metadata.Value
	return &value, nil
}
