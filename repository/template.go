package repository

import (
	"context"
	"fmt"

	"optionsbacktest/models"
)

func (r *Repository) ListTemplates(ctx context.Context, ownerID int64) ([]models.Strategy, error) {
	rows, err := r.db.QueryContext(ctx, QueryListTemplates, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()
	templates := make([]models.Strategy, 0)
	for rows.Next() {
		var strategy models.Strategy
		if err := rows.Scan(&strategy.ID, &strategy.Name, &strategy.Script, &strategy.CreatedAt, &strategy.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, strategy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate templates: %w", err)
	}
	return templates, nil
}
