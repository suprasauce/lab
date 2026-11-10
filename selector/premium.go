package selector

import (
	"context"
	"errors"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

type PremiumSelector struct{}

func (s PremiumSelector) Select(context.Context, int64, *models.Candle, time.Time, *repository.Repository) (*models.Option, error) {
	return nil, errors.New("premium selection is not implemented")
}
