package selector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"optionsbacktest/models"
	"optionsbacktest/pricing"
	"optionsbacktest/repository"
)

type DeltaSelector struct {
	OptionType models.OptionType
	Delta      float64
}

var errDeltaUnavailable = errors.New("delta unavailable")

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
		return nil, fmt.Errorf("resolve ATM contract: %w", err)
	}
	atmDelta, err := calculateContractDelta(ctx, atm, candle, repo)
	if err != nil {
		return nil, err
	}
	direction := 1
	if s.Delta > math.Abs(atmDelta) {
		direction = -1
	}
	if s.OptionType == models.Put {
		direction = -direction
	}
	contracts := []*models.Option{atm}
	for i := 1; i < 64; i++ {
		contract, err := NewOffsetSelector(s.OptionType, direction*i).Select(ctx, underlyingID, candle, expiry, repo)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("resolve contract at offset %d: %w", direction*i, err)
		}
		contracts = append(contracts, contract)
	}
	// Order by expected delta magnitude; historical IV may make deltas noisy.
	if s.Delta < math.Abs(atmDelta) {
		slices.Reverse(contracts)
	}
	return s.binarySearch(ctx, contracts, candle, repo)
}

func (s DeltaSelector) binarySearch(ctx context.Context, contracts []*models.Option, candle *models.Candle, repo *repository.Repository) (*models.Option, error) {
	if len(contracts) == 0 {
		return nil, fmt.Errorf("binary search: contract list is empty")
	}
	low, high := 0, len(contracts)-1
	for low+1 < high {
		_, err := calculateContractDelta(ctx, contracts[low], candle, repo)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errDeltaUnavailable) {
			low++
			continue
		}
		if err != nil {
			return nil, err
		}
		_, err = calculateContractDelta(ctx, contracts[high], candle, repo)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errDeltaUnavailable) {
			high--
			continue
		}
		if err != nil {
			return nil, err
		}
		mid := low + (high-low)/2
		left, right := mid, mid+1
		var leftDelta, rightDelta float64
		for left >= low || right <= high {
			if left >= low {
				value, candidateErr := calculateContractDelta(ctx, contracts[left], candle, repo)
				if errors.Is(candidateErr, sql.ErrNoRows) || errors.Is(candidateErr, errDeltaUnavailable) {
					left--
					continue
				}
				if candidateErr != nil {
					return nil, candidateErr
				}
				leftDelta = math.Abs(value)
			}
			if right <= high {
				value, candidateErr := calculateContractDelta(ctx, contracts[right], candle, repo)
				if errors.Is(candidateErr, sql.ErrNoRows) || errors.Is(candidateErr, errDeltaUnavailable) {
					right++
					continue
				}
				if candidateErr != nil {
					return nil, candidateErr
				}
				rightDelta = math.Abs(value)
			}
			break
		}
		var delta float64
		if left < low && right > high {
			break
		} else if left < low {
			mid, delta = right, rightDelta
		} else if right > high {
			mid, delta = left, leftDelta
		} else if math.Abs(leftDelta-s.Delta) <= math.Abs(rightDelta-s.Delta) {
			mid, delta = left, leftDelta
		} else {
			mid, delta = right, rightDelta
		}
		if mid == low || mid == high {
			break
		}
		delta = math.Abs(delta)
		if delta == s.Delta {
			return contracts[mid], nil
		}
		if delta < s.Delta {
			low = mid
		} else {
			high = mid
		}
	}
	var result *models.Option
	closest := math.Inf(1)
	for _, index := range []int{low, high} {
		delta, err := calculateContractDelta(ctx, contracts[index], candle, repo)
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errDeltaUnavailable) {
			continue
		}
		if err != nil {
			return nil, err
		}
		difference := math.Abs(math.Abs(delta) - s.Delta)
		if difference < closest {
			result, closest = contracts[index], difference
		}
	}
	if result == nil {
		return nil, fmt.Errorf("binary search: no valid delta found")
	}
	return result, nil
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
		return 0, fmt.Errorf("calculate delta for contract %d: %w: %w", contract.ID, errDeltaUnavailable, err)
	}
	return delta, nil
}
