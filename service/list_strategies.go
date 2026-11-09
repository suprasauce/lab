package service

import (
	"context"
	"strconv"
	"time"

	"optionsbacktest/repository"
)

type StrategyResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Script      string    `json:"script"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type StrategyListResponse struct {
	Strategies []StrategyResponse `json:"strategies"`
}

func ListStrategies(ctx context.Context, repo *repository.Repository, userID int64) (StrategyListResponse, error) {
	strategies, err := repo.ListStrategies(ctx, userID)
	if err != nil {
		return StrategyListResponse{}, err
	}
	response := StrategyListResponse{Strategies: make([]StrategyResponse, len(strategies))}
	for i, strategy := range strategies {
		response.Strategies[i] = StrategyResponse{
			ID:          strconv.FormatInt(strategy.ID, 10),
			Name:        strategy.Name,
			Description: strategy.Description,
			Script:      strategy.Script,
			CreatedAt:   strategy.CreatedAt,
			UpdatedAt:   strategy.UpdatedAt,
		}
	}
	return response, nil
}
