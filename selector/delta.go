package selector

import (
	"context"
	"errors"

	"optionsbacktest/models"
)

type DeltaSelector struct{}

func (s DeltaSelector) Select(ctx context.Context, input models.SelectionInput) (models.Option, error) {
	return models.Option{}, errors.New("delta selection is not implemented")
}
