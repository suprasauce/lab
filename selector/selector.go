package selector

import (
	"context"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

// Selector resolves an option contract at execution time, not at order placement.
// A nil contract leaves the order pending. The engine controls candle timing.
type Selector interface {
	Select(ctx context.Context, underlyingID int64, candle *models.Candle, expiry time.Time, repo *repository.Repository) (*models.Option, error)
}
