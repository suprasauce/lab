package chart

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
)

func (s *ChartService) EMA(ctx context.Context, tradingSymbol string, period int, timeframe models.Timeframe, at time.Time) (float64, error) {
	// TODO: Calculate EMA using completed candle closes available at simulation time.
	return 0, fmt.Errorf("EMA not implemented")
}
