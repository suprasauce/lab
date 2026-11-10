package selector

import (
	"context"
	"errors"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

type DeltaSelector struct {
	OptionType models.OptionType
	Delta      float64
}

func NewDeltaSelector(optionType models.OptionType, delta float64) *DeltaSelector {
	return &DeltaSelector{OptionType: optionType, Delta: delta}
}

func (s DeltaSelector) Select(context.Context, int64, *models.Candle, time.Time, *repository.Repository) (*models.Option, error) {
	// TODO: Implement delta-based contract selection.
	return nil, errors.New("delta selection is not implemented")
}
