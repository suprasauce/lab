package optionchain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"optionsbacktest/models"
)

const strikeInterval models.Price = 50 * 100 // INR 50 in paise.
const maxOffset = 40

func (s *OptionChainService) getSymbolByOffset(ctx context.Context, underlying string, expiry time.Time, optionType string, value float64, at time.Time) (string, error) {
	offset := int64(value)
	if float64(offset) != value {
		return "", fmt.Errorf("offset must be a whole number")
	}
	if offset < -maxOffset || offset > maxOffset {
		return "", fmt.Errorf("offset must be between -%d and %d", maxOffset, maxOffset)
	}
	asset, err := s.repo.GetUnderlying(ctx, underlying)
	if err != nil {
		return "", fmt.Errorf("offset selection: resolve underlying: %w", err)
	}
	candle, err := s.repo.GetLatestCompletedUnderlyingCandle(ctx, asset.ID, at)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("offset selection: fetch underlying candle: %w", err)
	}
	atm := ((candle.Open + strikeInterval/2) / strikeInterval) * strikeInterval
	strike := atm + models.Price(offset)*strikeInterval
	if strike <= 0 {
		return "", fmt.Errorf("offset selects a nonpositive strike")
	}
	kind := models.Call
	if optionType == "PE" {
		kind = models.Put
	}
	symbol, err := s.repo.GetOptionTradingSymbol(ctx, asset.ID, expiry, strike, kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("offset selection: resolve trading symbol: %w", err)
	}
	return symbol, nil
}
