// Package pricing implements European Black-Scholes pricing and delta.
// Monetary inputs must use the same unit (for example, rupees); rates and
// volatility are annual decimals, and time to expiry is expressed in years.
package pricing

import (
	"fmt"
	"math"
	"time"

	"optionsbacktest/models"
)

const (
	RiskFreeRate  = 0.06
	DividendYield = 0.0
	DaysPerYear   = 365.0
)

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func normalCDF(value float64) float64 {
	return 0.5 * math.Erfc(-value/math.Sqrt2)
}

func validateInputs(spot, strike, timeToExpiry float64, optionType models.OptionType) error {
	if optionType != models.Call && optionType != models.Put {
		return fmt.Errorf("invalid option type %q: expected CALL or PUT", optionType)
	}
	if !finite(spot) || spot <= 0 {
		return fmt.Errorf("spot must be finite and positive")
	}
	if !finite(strike) || strike <= 0 {
		return fmt.Errorf("strike must be finite and positive")
	}
	if !finite(timeToExpiry) || timeToExpiry <= 0 {
		return fmt.Errorf("time to expiry must be finite and positive")
	}
	return nil
}

func dValues(spot, strike, timeToExpiry, volatility float64, optionType models.OptionType) (float64, float64, error) {
	if err := validateInputs(spot, strike, timeToExpiry, optionType); err != nil {
		return 0, 0, err
	}
	if !finite(volatility) || volatility <= 0 {
		return 0, 0, fmt.Errorf("volatility must be finite and positive")
	}
	stdDev := volatility * math.Sqrt(timeToExpiry)
	// Separate logarithms avoid overflowing or underflowing spot/strike.
	d1 := (math.Log(spot)-math.Log(strike)+(RiskFreeRate-DividendYield)*timeToExpiry)/stdDev + stdDev/2
	d2 := d1 - stdDev
	if stdDev == 0 || !finite(d1) || !finite(d2) {
		return 0, 0, fmt.Errorf("Black-Scholes calculation exceeds numeric range")
	}
	return d1, d2, nil
}

func discountedValues(spot, strike, timeToExpiry float64) (float64, float64, error) {
	discountedSpot := spot * math.Exp(-DividendYield*timeToExpiry)
	discountedStrike := strike * math.Exp(-RiskFreeRate*timeToExpiry)
	if !finite(discountedSpot) || !finite(discountedStrike) || discountedSpot <= 0 || discountedStrike <= 0 {
		return 0, 0, fmt.Errorf("discounted prices exceed numeric range")
	}
	return discountedSpot, discountedStrike, nil
}

// BlackScholesPrice returns a European option premium in the inputs' currency unit.
// Zero time and zero volatility are rejected rather than using limiting payoffs.
func BlackScholesPrice(spot, strike, timeToExpiry, volatility float64, optionType models.OptionType) (float64, error) {
	d1, d2, err := dValues(spot, strike, timeToExpiry, volatility, optionType)
	if err != nil {
		return 0, err
	}
	s, k, err := discountedValues(spot, strike, timeToExpiry)
	if err != nil {
		return 0, err
	}
	price := s*normalCDF(d1) - k*normalCDF(d2)
	if optionType == models.Put {
		price = k*normalCDF(-d2) - s*normalCDF(-d1)
	}
	if !finite(price) {
		return 0, fmt.Errorf("option premium exceeds numeric range")
	}
	// Cancellation near the zero-price limit can produce a tiny negative value.
	return math.Max(price, 0), nil
}

// Delta returns spot delta. In extreme tails float64 may round it to 0 or +/-1.
func Delta(spot, strike, timeToExpiry, volatility float64, optionType models.OptionType) (float64, error) {
	d1, _, err := dValues(spot, strike, timeToExpiry, volatility, optionType)
	if err != nil {
		return 0, err
	}
	discount := math.Exp(-DividendYield * timeToExpiry)
	delta := discount * normalCDF(d1)
	if optionType == models.Put {
		// N(d1)-1 = -N(-d1); this avoids cancellation in the put's small-delta tail.
		delta = -discount * normalCDF(-d1)
	}
	if !finite(delta) {
		return 0, fmt.Errorf("option delta exceeds numeric range")
	}
	return delta, nil
}

// TimeToExpiry uses ACT/365 elapsed time. The caller supplies the actual expiry
// instant, including timezone and market closing time, rather than a date alone.
func TimeToExpiry(observationTime, expiryTime time.Time) (float64, error) {
	if !expiryTime.After(observationTime) {
		return 0, fmt.Errorf("expiry time must be after observation time")
	}
	duration := expiryTime.Sub(observationTime)
	if !observationTime.Add(duration).Equal(expiryTime) {
		return 0, fmt.Errorf("time to expiry exceeds time.Duration range")
	}
	return duration.Hours() / (24 * DaysPerYear), nil
}
