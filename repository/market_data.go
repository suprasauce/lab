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

// GetExpiries returns distinct expiry dates in ascending order within the inclusive range.
// The expiry range selects contracts, not the strategy's trading timestamps.
func (r *MarketDataRepository) GetExpiries(
	ctx context.Context,
	underlyingID int64,
	startExpiry time.Time,
	endExpiry time.Time,
) ([]time.Time, error) {
	if startExpiry.After(endExpiry) {
		return nil, fmt.Errorf("get expiries: start expiry must not be after end expiry")
	}

	r.logger.DebugContext(ctx, "fetching expiries",
		"underlying_id", underlyingID,
		"start_expiry", startExpiry,
		"end_expiry", endExpiry,
	)
	rows, err := r.db.QueryContext(ctx, queryGetExpiries, underlyingID, startExpiry, endExpiry)
	if err != nil {
		return nil, fmt.Errorf("query expiries: %w", err)
	}
	defer rows.Close()

	expiries := make([]time.Time, 0)
	for rows.Next() {
		var expiry time.Time
		if err := rows.Scan(&expiry); err != nil {
			return nil, fmt.Errorf("scan expiry: %w", err)
		}
		expiries = append(expiries, expiry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expiries: %w", err)
	}
	return expiries, nil
}

// GetOptionContract resolves a unique contract. Strike prices are in paise.
// If no contract matches, the returned error wraps sql.ErrNoRows.
func (r *MarketDataRepository) GetOptionContract(
	ctx context.Context,
	underlyingID int64,
	expiry time.Time,
	strikePrice models.Price,
	optionType models.OptionType,
) (models.Option, error) {
	r.logger.DebugContext(ctx, "fetching option contract",
		"underlying_id", underlyingID,
		"expiry", expiry,
		"strike_price", strikePrice,
		"option_type", optionType,
	)

	var contract models.Option
	err := r.db.QueryRowContext(ctx, queryGetOptionContract, underlyingID, expiry, strikePrice, optionType).Scan(
		&contract.ID,
		&contract.UnderlyingID,
		&contract.Expiry,
		&contract.StrikePrice,
		&contract.OptionType,
	)
	if err != nil {
		return models.Option{}, fmt.Errorf("get option contract: %w", err)
	}
	return contract, nil
}

// GetUnderlyingCandle returns the earliest available candle at or after startTime,
// including subsequent trading sessions. The caller validates its suitability.
// If no candle matches, the returned error wraps sql.ErrNoRows.
func (r *MarketDataRepository) GetUnderlyingCandle(
	ctx context.Context,
	underlyingID int64,
	startTime time.Time,
	timeframe models.Timeframe,
) (models.Candle, error) {
	r.logger.DebugContext(ctx, "fetching underlying candle",
		"underlying_id", underlyingID,
		"start_time", startTime,
		"timeframe", timeframe,
	)

	var candle models.Candle
	err := r.db.QueryRowContext(ctx, queryGetUnderlyingCandle, underlyingID, timeframe, startTime).Scan(
		&candle.Timestamp,
		&candle.Timeframe,
		&candle.Open,
		&candle.High,
		&candle.Low,
		&candle.Close,
		&candle.Volume,
	)
	if err != nil {
		return models.Candle{}, fmt.Errorf("get underlying candle: %w", err)
	}
	return candle, nil
}

// GetOptionCandles returns available candles in the inclusive timestamp interval,
// ordered chronologically. Missing candles are not filled or treated as errors.
func (r *MarketDataRepository) GetOptionCandles(
	ctx context.Context,
	contractID int64,
	startTime time.Time,
	endTime time.Time,
	timeframe models.Timeframe,
) ([]models.Candle, error) {
	if startTime.After(endTime) {
		return nil, fmt.Errorf("get option candles: start time must not be after end time")
	}

	r.logger.DebugContext(ctx, "fetching option candles",
		"contract_id", contractID,
		"start_time", startTime,
		"end_time", endTime,
		"timeframe", timeframe,
	)
	rows, err := r.db.QueryContext(ctx, queryGetOptionCandles, contractID, timeframe, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("query option candles: %w", err)
	}
	defer rows.Close()

	candles := make([]models.Candle, 0)
	for rows.Next() {
		var candle models.Candle
		if err := rows.Scan(
			&candle.Timestamp,
			&candle.Timeframe,
			&candle.Open,
			&candle.High,
			&candle.Low,
			&candle.Close,
			&candle.Volume,
		); err != nil {
			return nil, fmt.Errorf("scan option candle: %w", err)
		}
		candles = append(candles, candle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate option candles: %w", err)
	}
	return candles, nil
}
