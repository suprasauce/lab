package chart

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
)

type emaMetadata struct {
	Value       float64
	NextIndex   int
	Initialized bool
}

var marketLocation = time.FixedZone("Asia/Kolkata", 19800)

// EMA aggregates cached 1-minute history into completed session-aligned 5-minute closes.
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
		start := candles[metadata.NextIndex].Timestamp.In(marketLocation)
		minute := start.Hour()*60 + start.Minute()
		if minute < 9*60+15 || minute >= 15*60+30 {
			metadata.NextIndex++
			continue
		}
		start = time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), start.Minute()-start.Minute()%5, 0, 0, marketLocation)
		end := start.Add(5 * time.Minute)
		if end.After(at) {
			break
		}
		next := metadata.NextIndex
		for next < len(candles) && candles[next].Timestamp.Before(end) {
			next++
		}
		complete := next-metadata.NextIndex == 5
		for i := metadata.NextIndex; complete && i < next; i++ {
			complete = candles[i].Timestamp.Equal(start.Add(time.Duration(i-metadata.NextIndex) * time.Minute))
		}
		if complete {
			close := float64(candles[next-1].Close) / 100
			if !metadata.Initialized {
				metadata.Value = close
				metadata.Initialized = true
			} else {
				metadata.Value = close*alpha + metadata.Value*(1-alpha)
			}
		}
		metadata.NextIndex = next
	}
	if !metadata.Initialized {
		return nil, nil
	}
	value := metadata.Value
	return &value, nil
}
