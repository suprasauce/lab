package service

import (
	"context"

	"optionsbacktest/repository"
)

func SubmitBacktest(ctx context.Context, repo *repository.Repository, userID int64, script string) (int64, error) {
	return repo.CreateBacktest(ctx, userID, script)
}
