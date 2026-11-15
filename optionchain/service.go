package optionchain

import (
	"fmt"
	"time"
)

// OptionFilter describes a delta, strike, or price selection target.
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
	// TODO: Return the selected contract's trading symbol using data available at at.
	return "", fmt.Errorf("option chain contract selection is not implemented")
}
