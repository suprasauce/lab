package service

import (
	"context"

	"optionsbacktest/repository"
)

func UpdateStrategy(ctx context.Context, repo *repository.Repository, id, userID int64, name, script string) (int64, error) {
	return repo.UpdateStrategy(ctx, id, userID, name, script)
}
