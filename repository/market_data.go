package repository

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/models"
)

// GetExpiry offsets from the nearest stored expiry on or after the supplied date.
func (r *Repository) GetExpiry(ctx context.Context, underlying, expiryType string, at time.Time, offset int) (time.Time, error) {
	date := at.Format(time.DateOnly)
	r.logger.DebugContext(ctx, "fetching expiry", "underlying", underlying, "expiry_type", expiryType, "date", date, "offset", offset)
	var expiry time.Time
	if err := r.db.QueryRowContext(ctx, queryGetExpiry, underlying, expiryType, date, offset).Scan(&expiry); err != nil {
		return time.Time{}, fmt.Errorf("get expiry for %s from %s offset %d: %w", underlying, date, offset, err)
	}
	return expiry, nil
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

// OptionExpiryExists checks whether an expiry is registered for the symbol.
func (r *Repository) OptionExpiryExists(ctx context.Context, symbol string, expiry time.Time) (bool, error) {
	r.logger.DebugContext(ctx, "checking option expiry", "symbol", symbol, "expiry", expiry)
	var exists bool
	if err := r.db.QueryRowContext(ctx, queryOptionExpiryExists, symbol, expiry).Scan(&exists); err != nil {
		return false, fmt.Errorf("check option expiry for %s: %w", symbol, err)
	}
	return exists, nil
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
// Missing contracts return a wrapped sql.ErrNoRows.
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
	return r.queryOptionContract(ctx, underlyingID, expiry, strikePrice, optionType)
}

// GetOptionContractByTradingSymbol reads the stored symbol without ingestion.
func (r *Repository) GetOptionContractByTradingSymbol(ctx context.Context, tradingSymbol string) (models.Option, error) {
	r.logger.DebugContext(ctx, "fetching option contract by trading symbol", "tradingsymbol", tradingSymbol)
	var contract models.Option
	err := r.db.QueryRowContext(ctx, queryGetOptionContractByTradingSymbol, tradingSymbol).Scan(
		&contract.ID, &contract.UnderlyingID, &contract.ExpiryID, &contract.Expiry,
		&contract.StrikePrice, &contract.OptionType, &contract.UnderlyingSymbol, &contract.LotSize,
	)
	if err != nil {
		return models.Option{}, fmt.Errorf("get option contract for trading symbol %q: %w", tradingSymbol, err)
	}
	return contract, nil
}

// GetOptionTradingSymbol reads an existing contract's symbol without ingestion.
func (r *Repository) GetOptionTradingSymbol(ctx context.Context, underlyingID int64, expiry time.Time, strike models.Price, optionType models.OptionType) (string, error) {
	r.logger.DebugContext(ctx, "fetching option trading symbol", "underlying_id", underlyingID,
		"expiry", expiry, "strike_price", strike, "option_type", optionType)
	var symbol string
	if err := r.db.QueryRowContext(ctx, queryGetOptionTradingSymbol, underlyingID, expiry, strike, optionType).Scan(&symbol); err != nil {
		return "", fmt.Errorf("get option trading symbol: %w", err)
	}
	return symbol, nil
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
		&contract.UnderlyingSymbol,
		&contract.LotSize,
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

// GetLatestCompletedUnderlyingCandle returns the latest one-minute bar whose
// close time is at or before at. It never returns an unfinished candle.
func (r *Repository) GetLatestCompletedUnderlyingCandle(ctx context.Context, underlyingID int64, at time.Time) (models.Candle, error) {
	r.logger.DebugContext(ctx, "fetching latest completed underlying candle", "underlying_id", underlyingID, "at", at)
	var candle models.Candle
	if err := r.db.QueryRowContext(ctx, queryGetLatestCompletedUnderlyingCandle, underlyingID, at).Scan(
		&candle.Timestamp, &candle.Timeframe, &candle.Open, &candle.High, &candle.Low, &candle.Close, &candle.Volume,
	); err != nil {
		return models.Candle{}, fmt.Errorf("get latest completed underlying candle: %w", err)
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

// OptionDeltaCandidate pairs a contract with its Open premium at a single candle timestamp.
type OptionDeltaCandidate struct {
	TradingSymbol string
	StrikePrice   models.Price
	Premium       models.Price
}

func (r *Repository) GetOptionDeltaCandidates(ctx context.Context, underlyingID int64, expiry time.Time, optionType models.OptionType, candleTime time.Time) ([]OptionDeltaCandidate, error) {
	r.logger.DebugContext(ctx, "fetching delta candidates", "underlying_id", underlyingID, "expiry", expiry, "option_type", optionType, "candle_time", candleTime)
	rows, err := r.db.QueryContext(ctx, queryGetOptionDeltaCandidates, underlyingID, expiry, optionType, candleTime)
	if err != nil {
		return nil, fmt.Errorf("query delta candidates: %w", err)
	}
	defer rows.Close()
	candidates := make([]OptionDeltaCandidate, 0)
	for rows.Next() {
		var candidate OptionDeltaCandidate
		if err := rows.Scan(&candidate.TradingSymbol, &candidate.StrikePrice, &candidate.Premium); err != nil {
			return nil, fmt.Errorf("scan delta candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate delta candidates: %w", err)
	}
	return candidates, nil
}
