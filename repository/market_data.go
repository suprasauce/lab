package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"optionsbacktest/models"
)

type MarketDataRepository struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewMarketDataRepository(db *sql.DB, logger *slog.Logger) *MarketDataRepository {
	return &MarketDataRepository{
		db:     db,
		logger: logger.With("component", "market_data_repository"),
	}
}

func (r *MarketDataRepository) GetAllUnderlyings(ctx context.Context) ([]models.Underlying, error) {
	r.logger.DebugContext(ctx, "fetching all underlyings")
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
