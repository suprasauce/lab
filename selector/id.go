package selector

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

type IDSelector struct {
	ContractID int64
}

func NewIDSelector(contractID int64) *IDSelector {
	return &IDSelector{ContractID: contractID}
}

func (s *IDSelector) Select(ctx context.Context, underlyingID int64, _ *models.Candle, expiry time.Time, repo *repository.Repository) (*models.Option, error) {
	if s.ContractID <= 0 {
		return nil, fmt.Errorf("contract ID must be positive")
	}
	contract, err := repo.GetOptionContractByID(ctx, s.ContractID)
	if err != nil {
		return nil, fmt.Errorf("ID selection: %w", err)
	}
	if contract.UnderlyingID != underlyingID || contract.Expiry.Format(time.DateOnly) != expiry.Format(time.DateOnly) {
		return nil, fmt.Errorf("contract %d does not belong to the current underlying and expiry", contract.ID)
	}
	return &contract, nil
}
