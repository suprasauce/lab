package selector

import (
	"errors"

	"optionsbacktest/models"
)

type PremiumSelector struct{}

func (s PremiumSelector) SelectStrike(candle models.Candle) (models.Price, error) {
	return 0, errors.New("premium selection is not implemented")
}
