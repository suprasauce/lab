package service

import (
	"context"
	"strconv"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

type BacktestListItem struct {
	ID        string                `json:"id"`
	Status    models.BacktestStatus `json:"status"`
	CreatedAt time.Time             `json:"created_at"`
}

type BacktestListResponse struct {
	Backtests []BacktestListItem `json:"backtests"`
}

func ListBacktests(ctx context.Context, repo *repository.Repository, userID int64) (BacktestListResponse, error) {
	backtests, err := repo.ListBacktests(ctx, userID)
	if err != nil {
		return BacktestListResponse{}, err
	}
	response := BacktestListResponse{Backtests: make([]BacktestListItem, len(backtests))}
	for i, backtest := range backtests {
		response.Backtests[i] = BacktestListItem{
			ID:        strconv.FormatInt(backtest.ID, 10),
			Status:    backtest.Status,
			CreatedAt: backtest.CreatedAt,
		}
	}
	return response, nil
}
