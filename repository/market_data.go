package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"optionsbacktest/models"
)

func (r *Repository) GetAllUnderlyings(ctx context.Context) ([]models.Underlying, error) {
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

// GetUnderlying resolves a symbol. Missing symbols return a wrapped sql.ErrNoRows.
func (r *Repository) GetUnderlying(ctx context.Context, symbol string) (models.Underlying, error) {
	r.logger.DebugContext(ctx, "fetching underlying", "symbol", symbol)
	var underlying models.Underlying
	if err := r.db.QueryRowContext(ctx, queryGetUnderlying, symbol).Scan(&underlying.ID, &underlying.Symbol); err != nil {
		return models.Underlying{}, fmt.Errorf("get underlying %q: %w", symbol, err)
	}
	return underlying, nil
}

// GetExpiries returns distinct expiry dates in ascending order within the inclusive range.
// The expiry range selects contracts, not the strategy's trading timestamps.
func (r *Repository) GetExpiries(
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
// Missing or unsynced contracts have their full history ingested before returning.
func (r *Repository) GetOptionContract(
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
	contract, err := r.queryOptionContract(ctx, underlyingID, expiry, strikePrice, optionType)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract, err
	}
	if err == nil && contract.Synced {
		return contract, nil
	}

	var symbol string
	if err := r.db.QueryRowContext(ctx, queryGetUnderlyingSymbol, underlyingID).Scan(&symbol); err != nil {
		return models.Option{}, fmt.Errorf("resolve underlying symbol for %d: %w", underlyingID, err)
	}
	r.logger.DebugContext(ctx, "ingesting missing option contract", "symbol", symbol, "expiry", expiry,
		"strike_price", strikePrice, "option_type", optionType)
	if err := r.fetchOptionContract(ctx, symbol, expiry, strikePrice, optionType); err != nil {
		return models.Option{}, fmt.Errorf("fetch missing option contract: %w", err)
	}
	contract, err = r.queryOptionContract(ctx, underlyingID, expiry, strikePrice, optionType)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Option{}, fmt.Errorf("option contract still missing after successful ingestion: %w", err)
	}
	if err != nil {
		return models.Option{}, fmt.Errorf("query option contract after ingestion: %w", err)
	}
	if !contract.Synced {
		return models.Option{}, fmt.Errorf("option contract %d remains unsynced after ingestion", contract.ID)
	}
	return contract, nil
}

// GetOptionContractForDay resolves a candidate with only the requested day's history.
// It does not trigger full-history ingestion or mark a contract synced.
func (r *Repository) GetOptionContractForDay(ctx context.Context, underlyingID int64, expiry time.Time, strikePrice models.Price, optionType models.OptionType, day time.Time) (models.Option, error) {
	day = day.In(optionMarketLocation)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, optionMarketLocation)
	r.logger.DebugContext(ctx, "fetching option contract day", "underlying_id", underlyingID,
		"expiry", expiry, "strike_price", strikePrice, "option_type", optionType, "day", start)
	contract, err := r.queryOptionContract(ctx, underlyingID, expiry, strikePrice, optionType)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return models.Option{}, err
	}
	if err == nil {
		if contract.Synced {
			return contract, nil
		}
		var exists bool
		if err := r.db.QueryRowContext(ctx, queryHasOptionCandlesForDay, contract.ID, start, start.AddDate(0, 0, 1)).Scan(&exists); err != nil {
			return models.Option{}, fmt.Errorf("check option day candles: %w", err)
		}
		if exists {
			return contract, nil
		}
	}
	var symbol string
	if err := r.db.QueryRowContext(ctx, queryGetUnderlyingSymbol, underlyingID).Scan(&symbol); err != nil {
		return models.Option{}, fmt.Errorf("resolve underlying symbol for %d: %w", underlyingID, err)
	}
	if err := r.fetchOptionData(ctx, symbol, expiry, strikePrice, optionType, &start); err != nil {
		return models.Option{}, fmt.Errorf("fetch option day: %w", err)
	}
	return r.queryOptionContract(ctx, underlyingID, expiry, strikePrice, optionType)
}

var optionMarketLocation = time.FixedZone("IST", 5*60*60+30*60)

// GetOptionContractByID resolves an existing contract and syncs full history if needed.
func (r *Repository) GetOptionContractByID(ctx context.Context, contractID int64) (models.Option, error) {
	r.logger.DebugContext(ctx, "fetching option contract by ID", "contract_id", contractID)
	var contract models.Option
	if err := r.db.QueryRowContext(ctx, queryGetOptionContractByID, contractID).Scan(
		&contract.ID, &contract.UnderlyingID, &contract.ExpiryID, &contract.Expiry,
		&contract.StrikePrice, &contract.OptionType, &contract.Synced,
	); err != nil {
		return models.Option{}, fmt.Errorf("get option contract %d: %w", contractID, err)
	}
	if !contract.Synced {
		return r.GetOptionContract(ctx, contract.UnderlyingID, contract.Expiry, contract.StrikePrice, contract.OptionType)
	}
	return contract, nil
}

func (r *Repository) queryOptionContract(ctx context.Context, underlyingID int64, expiry time.Time, strikePrice models.Price, optionType models.OptionType) (models.Option, error) {
	var contract models.Option
	err := r.db.QueryRowContext(ctx, queryGetOptionContract, underlyingID, expiry, strikePrice, optionType).Scan(
		&contract.ID,
		&contract.UnderlyingID,
		&contract.ExpiryID,
		&contract.Expiry,
		&contract.StrikePrice,
		&contract.OptionType,
		&contract.Synced,
	)
	if err != nil {
		return models.Option{}, fmt.Errorf("get option contract: %w", err)
	}
	return contract, nil
}

// GetUnderlyingCandle returns the earliest available candle at or after startTime,
// including subsequent trading sessions. The caller validates its suitability.
// If no candle matches, the returned error wraps sql.ErrNoRows.
func (r *Repository) GetUnderlyingCandle(
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

// GetUnderlyingCandles returns candles in [startTime, endTime), ordered by timestamp.
// Missing candles return an empty, non-nil slice.
func (r *Repository) GetUnderlyingCandles(
	ctx context.Context,
	underlyingID int64,
	startTime time.Time,
	endTime time.Time,
	timeframe models.Timeframe,
) ([]models.Candle, error) {
	if startTime.After(endTime) {
		return nil, fmt.Errorf("get underlying candles: start time must not be after end time")
	}
	r.logger.DebugContext(ctx, "fetching underlying candles",
		"underlying_id", underlyingID,
		"start_time", startTime,
		"end_time", endTime,
		"timeframe", timeframe,
	)
	rows, err := r.db.QueryContext(ctx, queryGetUnderlyingCandles, underlyingID, timeframe, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("query underlying candles: %w", err)
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
			return nil, fmt.Errorf("scan underlying candle: %w", err)
		}
		candles = append(candles, candle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate underlying candles: %w", err)
	}
	return candles, nil
}

// GetOptionCandles returns available candles in the inclusive timestamp interval,
// ordered chronologically. Missing candles are not filled or treated as errors.
func (r *Repository) GetOptionCandles(
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
