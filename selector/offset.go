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
	OptionType models.OptionType
	// StrikeOffset counts intervals above (positive) or below (negative) ATM.
	StrikeOffset int
}

func NewOffsetSelector(optionType models.OptionType, strikeOffset int) *OffsetSelector {
	return &OffsetSelector{OptionType: optionType, StrikeOffset: strikeOffset}
}

func (s *OffsetSelector) Select(ctx context.Context, underlyingID int64, candle *models.Candle, expiry time.Time, repo *repository.Repository) (*models.Option, error) {
	if s.OptionType != models.Call && s.OptionType != models.Put {
		return nil, fmt.Errorf("invalid option_type %q: expected CALL or PUT", s.OptionType)
	}
	if candle == nil {
		return nil, nil
	}
	strike, err := selectStrike(candle.Open, s.StrikeOffset)
	if err != nil {
		return nil, err
	}
	contract, err := repo.GetOptionContract(ctx, underlyingID, expiry, strike, s.OptionType)
	if err != nil {
		return nil, fmt.Errorf("offset selection: %w", err)
	}
	return &contract, nil
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
