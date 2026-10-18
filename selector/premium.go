package selector

import (
	"context"
	"errors"

	"optionsbacktest/models"
)

type PremiumSelector struct{}

func (s PremiumSelector) Select(ctx context.Context, input models.SelectionInput) (models.Option, error) {
	return models.Option{}, errors.New("premium selection is not implemented")
}
