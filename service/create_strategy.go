package service

import (
	"context"

	"optionsbacktest/repository"
)

func CreateStrategy(ctx context.Context, repo *repository.Repository, userID int64, name, script, description string) (int64, error) {
	return repo.CreateStrategy(ctx, userID, name, script, description)
}
