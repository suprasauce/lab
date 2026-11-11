package selector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

func (s DeltaSelector) Select(context.Context, int64, *models.Candle, time.Time, *repository.Repository) (*models.Option, error) {
	// TODO: Implement delta-based contract selection.
	return nil, errors.New("delta selection is not implemented")
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
