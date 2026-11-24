package optionchain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"optionsbacktest/models"
)

var marketLocation = time.FixedZone("Asia/Kolkata", 5*60*60+30*60)

func (s *OptionChainService) getSymbolByDelta(ctx context.Context, underlying string, expiry time.Time, optionType string, target float64, at time.Time) (string, error) {
	if !finite(target) || target < 0.10 || target > 1 {
		return "", fmt.Errorf("target delta must be between 0.10 and 1 inclusive")
	}
	asset, err := s.repo.GetUnderlying(ctx, underlying)
	if err != nil {
		return "", fmt.Errorf("delta selection: resolve underlying: %w", err)
	}
	candle, err := s.repo.GetLatestCompletedUnderlyingCandle(ctx, asset.ID, at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("delta selection: fetch underlying candle: %w", err)
	}
	// Both close prices are observed at the end of this completed one-minute bar.
	observedAt := candle.Timestamp.Add(time.Minute)
	expiresAt := time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 15, 30, 0, 0, marketLocation)
	if !observedAt.Before(expiresAt) {
		return "", nil
	}
	years, err := TimeToExpiry(observedAt, expiresAt)
	if err != nil {
		return "", err
	}
	kind := models.Call
	if optionType == "PE" {
		kind = models.Put
	}
	candidates, err := s.repo.GetOptionDeltaCandidates(ctx, asset.ID, expiry, kind, candle.Timestamp)
	if err != nil {
		return "", fmt.Errorf("delta selection: %w", err)
	}
	symbol, closest := "", math.Inf(1)
	for _, candidate := range candidates {
		delta, err := DeltaFromPremium(float64(candle.Close)/100, float64(candidate.StrikePrice)/100, float64(candidate.Premium)/100, years, kind)
		if err != nil {
			continue
		}
		difference := math.Abs(math.Abs(delta) - target)
		if difference < closest {
			symbol, closest = candidate.TradingSymbol, difference
		}
	}
	return symbol, nil
}
