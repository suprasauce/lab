package selector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/pricing"
	"optionsbacktest/repository"
)

type DeltaSelector struct {
	OptionType models.OptionType
	Delta      float64
}

var marketLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		panic(err)
	}
	return location
}()

func NewDeltaSelector(optionType models.OptionType, delta float64) *DeltaSelector {
	return &DeltaSelector{OptionType: optionType, Delta: delta}
}

func (s DeltaSelector) Select(ctx context.Context, underlyingID int64, candle *models.Candle, expiry time.Time, repo *repository.Repository) (*models.Option, error) {
	atm, err := NewOffsetSelector(s.OptionType, 0).Select(ctx, underlyingID, candle, expiry, repo)
	if err != nil {
		return nil, fmt.Errorf("delta selection: resolve ATM contract: %w", err)
	}
	atmDelta, err := calculateContractDelta(ctx, atm, candle, repo)
	if err != nil {
		return nil, fmt.Errorf("delta selection: ATM delta: %w", err)
	}
	direction := 1
	if s.Delta > math.Abs(atmDelta) {
		direction = -1
	}
	if s.OptionType == models.Put {
		direction = -direction
	}
	lower, upper, err := s.exponentialSearch(ctx, underlyingID, candle, expiry, repo, direction, atm)
	if err != nil {
		return nil, fmt.Errorf("delta selection: exponential search: %w", err)
	}
	contract, err := s.binarySearch(ctx, lower, upper, candle, repo)
	if err != nil {
		return nil, fmt.Errorf("delta selection: binary search: %w", err)
	}
	return contract, nil
}

// direction is +1 for increasing strikes and -1 for decreasing strikes.
func (s DeltaSelector) exponentialSearch(ctx context.Context, underlyingID int64, candle *models.Candle, expiry time.Time, repo *repository.Repository, direction int, atm *models.Option) (*models.Option, *models.Option, error) {
	const maxOffset = 64
	previous := atm
	decreasingDelta := (s.OptionType == models.Call && direction == 1) || (s.OptionType == models.Put && direction == -1)
	for step := 1; step <= maxOffset; step *= 2 {
		contract, err := NewOffsetSelector(s.OptionType, direction*step).Select(ctx, underlyingID, candle, expiry, repo)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve option at offset %d: %w", direction*step, err)
		}
		delta, err := calculateContractDelta(ctx, contract, candle, repo)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		delta = math.Abs(delta)
		if (decreasingDelta && delta <= s.Delta) || (!decreasingDelta && delta >= s.Delta) {
			if previous.StrikePrice < contract.StrikePrice {
				return previous, contract, nil
			}
			return contract, previous, nil
		}
		previous = contract
	}
	return nil, nil, fmt.Errorf("target delta %g not reached within %d strike intervals", s.Delta, maxOffset)
}
func (s DeltaSelector) binarySearch(ctx context.Context, lower, upper *models.Option, candle *models.Candle, repo *repository.Repository) (*models.Option, error) {
	// TODO: Refine the bracket and return the contract closest to the target delta.
	return nil, errors.New("binary search is not implemented")
}

func calculateContractDelta(ctx context.Context, contract *models.Option, underlyingCandle *models.Candle, repo *repository.Repository) (float64, error) {
	if contract == nil || underlyingCandle == nil {
		return 0, fmt.Errorf("calculate contract delta: contract and underlying candle are required")
	}
	at := underlyingCandle.Timestamp
	candles, err := repo.GetOptionCandles(ctx, contract.ID, at, at, models.Timeframe5m)
	if err != nil {
		return 0, fmt.Errorf("calculate delta for contract %d: fetch option candle: %w", contract.ID, err)
	}
	if len(candles) == 0 || !candles[0].Timestamp.Equal(at) {
		return 0, fmt.Errorf("calculate delta for contract %d: no option candle at %s: %w", contract.ID, at.Format(time.RFC3339), sql.ErrNoRows)
	}
	// The database expiry is a calendar date; expiration is at 15:30 IST.
	expiry := contract.Expiry
	expiresAt := time.Date(expiry.Year(), expiry.Month(), expiry.Day(), 15, 30, 0, 0, marketLocation)
	years, err := pricing.TimeToExpiry(candles[0].Timestamp, expiresAt)
	if err != nil {
		return 0, fmt.Errorf("calculate delta for contract %d: time to expiry: %w", contract.ID, err)
	}
	// Convert all monetary inputs from integer paise to rupees.
	delta, err := pricing.DeltaFromPremium(float64(underlyingCandle.Open)/100, float64(contract.StrikePrice)/100, float64(candles[0].Open)/100, years, contract.OptionType)
	if err != nil {
		return 0, fmt.Errorf("calculate delta for contract %d: %w", contract.ID, err)
	}
	return delta, nil
}
