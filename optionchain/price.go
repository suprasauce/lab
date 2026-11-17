package optionchain

import (
	"fmt"
	"time"
)

func (s *OptionChainService) getSymbolByPrice(underlying string, expiry time.Time, optionType string, value float64, at time.Time) (string, error) {
	// TODO: Select the contract closest to the target premium.
	return "", fmt.Errorf("price selection is not implemented")
}
