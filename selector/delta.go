package selector

import (
	"errors"

	"optionsbacktest/models"
)

type DeltaSelector struct{}

func (s DeltaSelector) SelectStrike(candle models.Candle) (models.Price, error) {
	return 0, errors.New("delta selection is not implemented")
}
