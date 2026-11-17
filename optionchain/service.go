package optionchain

import (
	"context"
	"fmt"
	"time"

	"optionsbacktest/repository"
)

// OptionFilter describes a delta, offset, or price selection target.
type OptionFilter struct {
	Type  string
	Value float64
}

// OptionChainService selects contracts without retaining backtest state.
type OptionChainService struct {
	repo *repository.Repository
}

func NewOptionChainService(repo *repository.Repository) *OptionChainService {
	return &OptionChainService{repo: repo}
}

// GetSymbol accepts an underlying symbol and CE or PE option type.
// Market data used for selection must be available at the simulation timestamp.
func (s *OptionChainService) GetSymbol(
	ctx context.Context,
	underlying string,
	expiry time.Time,
	optionType string,
	filter OptionFilter,
	at time.Time,
) (string, error) {
	if err := s.repo.EnsureOptionChain(ctx, underlying, expiry); err != nil {
		return "", fmt.Errorf("ensure option chain: %w", err)
	}
	switch filter.Type {
	case "delta":
		return s.getSymbolByDelta(ctx, underlying, expiry, optionType, filter.Value, at)
	case "offset":
		return s.getSymbolByOffset(ctx, underlying, expiry, optionType, filter.Value, at)
	case "price":
		return s.getSymbolByPrice(underlying, expiry, optionType, filter.Value, at)
	default:
		return "", fmt.Errorf("unsupported option filter type: %s", filter.Type)
	}
}
