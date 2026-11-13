package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"optionsbacktest/models"
	"optionsbacktest/repository"
)

type BacktestResponse struct {
	ID      string                `json:"id"`
	Status  models.BacktestStatus `json:"status"`
	Script  string                `json:"script"`
	Summary json.RawMessage       `json:"summary,omitempty"`
	Trades  json.RawMessage       `json:"trades,omitempty"`
	Error   string                `json:"error,omitempty"`
}

func GetBacktest(ctx context.Context, repo *repository.Repository, id, userID int64) (BacktestResponse, error) {
	backtest, err := repo.GetBacktestForUser(ctx, id, userID)
	if err != nil {
		return BacktestResponse{}, err
	}
	response := BacktestResponse{ID: strconv.FormatInt(backtest.ID, 10), Status: backtest.Status, Script: backtest.Script}
	switch backtest.Status {
	case models.BacktestPending:
		return response, nil
	case models.BacktestCompleted:
		var results struct {
			Summary json.RawMessage `json:"summary"`
			Trades  json.RawMessage `json:"trades"`
		}
		if err := json.Unmarshal(backtest.Results, &results); err != nil {
			return BacktestResponse{}, fmt.Errorf("decode backtest %d results: %w", id, err)
		}
		response.Summary, response.Trades = results.Summary, results.Trades
		return response, nil
	case models.BacktestFailed:
		var results struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(backtest.Results, &results); err != nil {
			return BacktestResponse{}, fmt.Errorf("decode backtest %d results: %w", id, err)
		}
		if results.Error == "" {
			return BacktestResponse{}, fmt.Errorf("backtest %d has no failure details", id)
		}
		response.Error = results.Error
		return response, nil
	default:
		return BacktestResponse{}, fmt.Errorf("unsupported backtest status: %s", backtest.Status)
	}
}
