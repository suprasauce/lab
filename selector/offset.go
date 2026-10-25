package selector

import (
	"fmt"
	"math"

	"optionsbacktest/models"
)

const strikeInterval models.Price = 50 * 100 // INR 50 in paise.

type OffsetSelector struct {
	// StrikeOffset counts intervals above (positive) or below (negative) ATM.
	StrikeOffset int
}

func NewOffsetSelector(strikeOffset int) *OffsetSelector {
	return &OffsetSelector{StrikeOffset: strikeOffset}
}

func (s *OffsetSelector) SelectStrike(candle models.Candle) (models.Price, error) {
	return selectStrike(candle.Open, s.StrikeOffset)
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
