package pricing

import (
	"fmt"
	"math"

	"optionsbacktest/models"
)

const (
	minVolatility = 1e-8
	maxVolatility = 5.0 // 500% annualized.
	maxIterations = 100
)

// ImpliedVolatility returns annual decimal IV by bisection in [1e-8, 5].
// Price tolerance is max(1e-10, spot*1e-12) in the inputs' currency unit.
// Premiums must be strictly inside the theoretical bounds: boundary prices
// cannot uniquely identify a finite positive IV at float64 precision.
func ImpliedVolatility(spot, strike, marketPremium, timeToExpiry float64, optionType models.OptionType) (float64, error) {
	if err := validateInputs(spot, strike, timeToExpiry, optionType); err != nil {
		return 0, err
	}
	s, k, err := discountedValues(spot, strike, timeToExpiry)
	if err != nil {
		return 0, err
	}
	lower, upper := math.Max(s-k, 0), s
	if optionType == models.Put {
		lower, upper = math.Max(k-s, 0), k
	}
	if !finite(marketPremium) || marketPremium <= lower || marketPremium >= upper {
		return 0, fmt.Errorf("premium must be finite and strictly between no-arbitrage bounds %g and %g", lower, upper)
	}
	low, high := minVolatility, maxVolatility
	lowPrice, err := BlackScholesPrice(spot, strike, timeToExpiry, low, optionType)
	if err != nil {
		return 0, err
	}
	highPrice, err := BlackScholesPrice(spot, strike, timeToExpiry, high, optionType)
	if err != nil {
		return 0, err
	}
	if marketPremium < lowPrice || marketPremium > highPrice {
		return 0, fmt.Errorf("no implied volatility solution in supported range [%g, %g]", minVolatility, maxVolatility)
	}
	tolerance := math.Max(1e-10, spot*1e-12)
	for range maxIterations {
		mid := (low + high) / 2
		price, err := BlackScholesPrice(spot, strike, timeToExpiry, mid, optionType)
		if err != nil {
			return 0, err
		}
		if math.Abs(price-marketPremium) <= tolerance {
			return mid, nil
		}
		if price < marketPremium {
			low = mid
		} else {
			high = mid
		}
	}
	return 0, fmt.Errorf("implied volatility did not converge after %d iterations", maxIterations)
}

// DeltaFromPremium infers IV from the premium and returns the corresponding delta.
func DeltaFromPremium(spot, strike, marketPremium, timeToExpiry float64, optionType models.OptionType) (float64, error) {
	volatility, err := ImpliedVolatility(spot, strike, marketPremium, timeToExpiry, optionType)
	if err != nil {
		return 0, fmt.Errorf("delta from premium: %w", err)
	}
	return Delta(spot, strike, timeToExpiry, volatility, optionType)
}
