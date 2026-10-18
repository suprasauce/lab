package repository

import (
	"context"
	"database/sql"
	"fmt"

	"optionsbacktest/models"
)

type MarketDataRepository struct {
	db *sql.DB
}

func NewMarketDataRepository(db *sql.DB) *MarketDataRepository {
	return &MarketDataRepository{db: db}
}

func (r *MarketDataRepository) GetAllUnderlyings(ctx context.Context) ([]models.Underlying, error) {
	rows, err := r.db.QueryContext(ctx, queryGetAllUnderlyings)
	if err != nil {
		return nil, fmt.Errorf("query all underlyings: %w", err)
	}
	defer rows.Close()

	underlyings := make([]models.Underlying, 0)
	for rows.Next() {
		var underlying models.Underlying
		if err := rows.Scan(&underlying.ID, &underlying.Symbol); err != nil {
			return nil, fmt.Errorf("scan underlying: %w", err)
		}
		underlyings = append(underlyings, underlying)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate underlyings: %w", err)
	}
	return underlyings, nil
}
