package selector

import (
	"context"
	"errors"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

type DeltaSelector struct{}

func (s DeltaSelector) Select(context.Context, int64, *models.Candle, time.Time, *repository.Repository) (*models.Option, error) {
	return nil, errors.New("delta selection is not implemented")
}
