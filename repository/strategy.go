package repository

import (
	"context"
	"fmt"

	"optionsbacktest/models"
)

func (r *Repository) CreateStrategy(ctx context.Context, userID int64, name, script string) (int64, error) {
	var id int64
	if err := r.db.QueryRowContext(ctx, QueryCreateStrategy, userID, name, script).Scan(&id); err != nil {
		return 0, fmt.Errorf("create strategy: %w", err)
	}
	return id, nil
}

func (r *Repository) ListStrategies(ctx context.Context, userID int64) ([]models.Strategy, error) {
	rows, err := r.db.QueryContext(ctx, QueryListStrategies, userID)
	if err != nil {
		return nil, fmt.Errorf("list strategies: %w", err)
	}
	defer rows.Close()
	strategies := make([]models.Strategy, 0)
	for rows.Next() {
		var strategy models.Strategy
		if err := rows.Scan(&strategy.ID, &strategy.Name, &strategy.Script, &strategy.CreatedAt, &strategy.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan strategy: %w", err)
		}
		strategies = append(strategies, strategy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate strategies: %w", err)
	}
	return strategies, nil
}
