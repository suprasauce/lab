package optionchain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

const strikeInterval models.Price = 50 * 100 // INR 50 in paise.
const maxOffset = 40

// OptionFilter describes a delta, offset, or price selection target.
type OptionFilter struct {
	Type  string
	Value float64
}

// OptionChainService selects contracts without retaining backtest state.
type OptionChainService struct {
	repo *repository.Repository
}

func NewOptionChainService(repo *repository.Repository) *OptionChainService {
	return &OptionChainService{repo: repo}
}

// GetSymbol accepts an underlying symbol and CE or PE option type.
// Market data used for selection must be available at the simulation timestamp.
func (s *OptionChainService) GetSymbol(
	ctx context.Context,
	underlying string,
	expiry time.Time,
	optionType string,
	filter OptionFilter,
	at time.Time,
) (string, error) {
	if err := s.repo.EnsureOptionChain(ctx, underlying, expiry); err != nil {
		return "", fmt.Errorf("ensure option chain: %w", err)
	}
	switch filter.Type {
	case "delta":
		return s.getSymbolByDelta(underlying, expiry, optionType, filter.Value, at)
	case "offset":
		return s.getSymbolByOffset(ctx, underlying, expiry, optionType, filter.Value, at)
	case "price":
		return s.getSymbolByPrice(underlying, expiry, optionType, filter.Value, at)
	default:
		return "", fmt.Errorf("unsupported option filter type: %s", filter.Type)
	}
}

func (s *OptionChainService) getSymbolByDelta(underlying string, expiry time.Time, optionType string, value float64, at time.Time) (string, error) {
	// TODO: Select the contract closest to the target delta.
	return "", fmt.Errorf("delta selection is not implemented")
}

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

func (s *OptionChainService) getSymbolByPrice(underlying string, expiry time.Time, optionType string, value float64, at time.Time) (string, error) {
	// TODO: Select the contract closest to the target premium.
	return "", fmt.Errorf("price selection is not implemented")
}
