package repository

import (
	"context"
	"database/sql"
	"fmt"

	"optionsbacktest/models"
)

func (r *Repository) GetPendingBacktestIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, QueryPendingBacktestIDs)
	if err != nil {
		return nil, fmt.Errorf("get pending backtest IDs: %w", err)
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan pending backtest ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending backtest IDs: %w", err)
	}
	return ids, nil
}

func (r *Repository) ListBacktests(ctx context.Context, userID int64) ([]models.Backtest, error) {
	rows, err := r.db.QueryContext(ctx, QueryListBacktests, userID)
	if err != nil {
		return nil, fmt.Errorf("list backtests: %w", err)
	}
	defer rows.Close()
	backtests := make([]models.Backtest, 0)
	for rows.Next() {
		var backtest models.Backtest
		if err := rows.Scan(&backtest.ID, &backtest.Status, &backtest.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan backtest: %w", err)
		}
		backtests = append(backtests, backtest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backtests: %w", err)
	}
	return backtests, nil
}

func (r *Repository) CreateBacktest(ctx context.Context, userID int64, script string) (int64, error) {
	var id int64
	if err := r.db.QueryRowContext(ctx, QueryCreateBacktest, userID, script).Scan(&id); err != nil {
		return 0, fmt.Errorf("create backtest: %w", err)
	}
	return id, nil
}

func (r *Repository) GetBacktest(ctx context.Context, id int64) (models.Backtest, error) {
	var backtest models.Backtest
	if err := r.db.QueryRowContext(ctx, QueryGetBacktest, id).Scan(&backtest.ID, &backtest.Script); err != nil {
		return models.Backtest{}, fmt.Errorf("get backtest %d: %w", id, err)
	}
	return backtest, nil
}

func (r *Repository) GetBacktestForUser(ctx context.Context, id, userID int64) (models.Backtest, error) {
	var backtest models.Backtest
	var results []byte
	if err := r.db.QueryRowContext(ctx, QueryGetBacktestForUser, id, userID).Scan(&backtest.ID, &backtest.Status, &results, &backtest.Script); err != nil {
		return models.Backtest{}, fmt.Errorf("get backtest %d for user: %w", id, err)
	}
	backtest.Results = results
	return backtest, nil
}

func (r *Repository) UpdateBacktest(ctx context.Context, id int64, status models.BacktestStatus, results []byte) error {
	result, err := r.db.ExecContext(ctx, QueryUpdateBacktest, id, status, string(results))
	if err != nil {
		return fmt.Errorf("update backtest %d: %w", id, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update backtest %d affected rows: %w", id, err)
	}
	if count == 0 {
		return fmt.Errorf("update backtest %d: %w", id, sql.ErrNoRows)
	}
	return nil
}
