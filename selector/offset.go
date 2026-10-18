package selector

import (
	"context"
	"fmt"
	"math"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

const strikeInterval models.Price = 50 * 100 // INR 50 in paise.

type OffsetSelector struct {
	// StrikeOffset counts intervals above (positive) or below (negative) ATM.
	StrikeOffset int
	repository   *repository.MarketDataRepository
}

func NewOffsetSelector(repo *repository.MarketDataRepository, strikeOffset int) *OffsetSelector {
	return &OffsetSelector{StrikeOffset: strikeOffset, repository: repo}
}

func (s *OffsetSelector) Select(ctx context.Context, input models.SelectionInput) (models.Option, error) {
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return models.Option{}, fmt.Errorf("offset selection: load timezone: %w", err)
	}
	// Expiry is a DATE: include its entire Kolkata calendar day. Later sessions
	// after the target are allowed, provided they remain inside this window.
	year, month, day := input.Expiry.Date()
	windowEnd := time.Date(year, month, day, 0, 0, 0, 0, location).AddDate(0, 0, 1)
	if !input.EntryTime.Before(windowEnd) {
		return models.Option{}, fmt.Errorf("offset selection: entry target is after expiry")
	}
	candle, err := s.repository.GetUnderlyingCandle(ctx, input.UnderlyingID, input.EntryTime, input.Timeframe)
	if err != nil {
		return models.Option{}, fmt.Errorf("offset selection: fetch underlying candle: %w", err)
	}
	if candle.Timestamp.Before(input.EntryTime) || !candle.Timestamp.Before(windowEnd) {
		return models.Option{}, fmt.Errorf("offset selection: underlying candle is outside the entry-to-expiry window")
	}
	strike, err := selectStrike(candle.Open, s.StrikeOffset)
	if err != nil {
		return models.Option{}, fmt.Errorf("offset selection: %w", err)
	}
	contract, err := s.repository.GetOptionContract(ctx, input.UnderlyingID, input.Expiry, strike, input.OptionType)
	if err != nil {
		return models.Option{}, fmt.Errorf("offset selection: resolve option contract: %w", err)
	}
	return contract, nil
}

func selectStrike(open models.Price, offset int) (models.Price, error) {
	if open < 0 {
		return 0, fmt.Errorf("underlying open price must be nonnegative")
	}
	// Integer paise avoid rounding through float64. Exact half intervals round up.
	atm := open / strikeInterval
	if open%strikeInterval >= strikeInterval/2 {
		atm++
	}
	shift := models.Price(offset)
	if shift < -atm {
		return 0, fmt.Errorf("strike offset produces a negative strike")
	}
	if shift > models.Price(math.MaxInt64)/strikeInterval-atm {
		return 0, fmt.Errorf("selected strike exceeds Price range")
	}
	return (atm + shift) * strikeInterval, nil
}
