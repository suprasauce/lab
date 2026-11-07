package service

import (
	"context"
	"optionsbacktest/repository"
	"optionsbacktest/worker"
)

func SubmitBacktest(ctx context.Context, repo *repository.Repository, pool *worker.WorkerPool, userID int64, script string) (int64, error) {
	id, err := repo.CreateBacktest(ctx, userID, script)
	if err != nil {
		return 0, err
	}
	pool.SubmitJob(id)
	return id, nil
}
