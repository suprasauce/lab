package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

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

// GetOptionContracts returns contracts in the inclusive expiry date range.
// The expiry range selects contracts, not the strategy's trading timestamps.
func (r *MarketDataRepository) GetOptionContracts(
	ctx context.Context,
	underlyingID int64,
	optionType models.OptionType,
	startExpiry time.Time,
	endExpiry time.Time,
) ([]models.Option, error) {
	if startExpiry.After(endExpiry) {
		return nil, fmt.Errorf("get option contracts: start expiry must not be after end expiry")
	}

	r.logger.DebugContext(ctx, "fetching option contracts",
		"underlying_id", underlyingID,
		"option_type", optionType,
		"start_expiry", startExpiry,
		"end_expiry", endExpiry,
	)
	rows, err := r.db.QueryContext(ctx, queryGetOptionContracts, underlyingID, optionType, startExpiry, endExpiry)
	if err != nil {
		return nil, fmt.Errorf("query option contracts: %w", err)
	}
	defer rows.Close()

	contracts := make([]models.Option, 0)
	for rows.Next() {
		var contract models.Option
		if err := rows.Scan(
			&contract.ID,
			&contract.UnderlyingID,
			&contract.Expiry,
			&contract.StrikePrice,
			&contract.OptionType,
		); err != nil {
			return nil, fmt.Errorf("scan option contract: %w", err)
		}
		contracts = append(contracts, contract)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate option contracts: %w", err)
	}
	return contracts, nil
}
