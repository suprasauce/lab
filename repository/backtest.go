package repository

import (
	"context"
	"fmt"
)

func (r *Repository) CreateBacktest(ctx context.Context, userID int64, script string) (int64, error) {
	var id int64
	if err := r.db.QueryRowContext(ctx, queryCreateBacktest, userID, script).Scan(&id); err != nil {
		return 0, fmt.Errorf("create backtest: %w", err)
	}
	return id, nil
}
