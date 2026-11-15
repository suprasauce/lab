package optionchain

import (
	"fmt"
	"time"
)

// OptionFilter describes a delta, offset, or price selection target.
type OptionFilter struct {
	Type  string
	Value float64
}

// OptionChainService selects contracts without retaining backtest state.
type OptionChainService struct{}

func NewOptionChainService() *OptionChainService {
	return &OptionChainService{}
}

// GetSymbol accepts an underlying symbol and CE or PE option type.
// Market data used for selection must be available at the simulation timestamp.
func (s *OptionChainService) GetSymbol(
	underlying string,
	expiry time.Time,
	optionType string,
	filter OptionFilter,
	at time.Time,
) (string, error) {
	switch filter.Type {
	case "delta":
		return s.getSymbolByDelta(underlying, expiry, optionType, filter.Value, at)
	case "offset":
		return s.getSymbolByOffset(underlying, expiry, optionType, filter.Value, at)
	case "price":
		return s.getSymbolByPrice(underlying, expiry, optionType, filter.Value, at)
	default:
		return "", fmt.Errorf("unsupported option filter type: %s", filter.Type)
	}
}

func (s *OptionChainService) getSymbolByDelta(underlying string, expiry time.Time, optionType string, value float64, at time.Time) (string, error) {
	// TODO: Select the contract closest to the target delta.
	return "", fmt.Errorf("delta selection is not implemented")
}

func (s *OptionChainService) getSymbolByOffset(underlying string, expiry time.Time, optionType string, value float64, at time.Time) (string, error) {
	// TODO: Select the contract at the requested ATM strike offset.
	return "", fmt.Errorf("offset selection is not implemented")
}

func (s *OptionChainService) getSymbolByPrice(underlying string, expiry time.Time, optionType string, value float64, at time.Time) (string, error) {
	// TODO: Select the contract closest to the target premium.
	return "", fmt.Errorf("price selection is not implemented")
}
