package models

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// OptionSymbol contains the contract attributes encoded in a Lab trading symbol.
type OptionSymbol struct {
	Underlying  string
	Expiry      time.Time
	StrikePrice Price
	OptionType  OptionType
}

// GenerateOptionTradingSymbol uses rupees in the symbol and paise internally.
func GenerateOptionTradingSymbol(underlying string, expiry time.Time, strike Price, optionType OptionType) string {
	suffix := "CE"
	if optionType == Put {
		suffix = "PE"
	}
	amount := strconv.FormatInt(int64(strike)/100, 10)

	return strings.ToUpper(underlying) + "-" + expiry.Format("20060102") + "-" + amount + "-" + suffix
}

// ParseOptionTradingSymbol accepts the canonical Lab format, never broker symbols.
func ParseOptionTradingSymbol(symbol string) (OptionSymbol, error) {
	invalid := func() (OptionSymbol, error) {
		return OptionSymbol{}, fmt.Errorf("invalid Lab trading symbol %q: expected UNDERLYING-YYYYMMDD-STRIKE-CE/PE", symbol)
	}
	parts := strings.Split(symbol, "-")
	if len(parts) != 4 {
		return invalid()
	}
	expiry, err := time.Parse("20060102", parts[1])
	if err != nil || expiry.Year() < 1 {
		return invalid()
	}
	if valid, _ := regexp.MatchString(`^(0|[1-9][0-9]*)$`, parts[2]); !valid {
		return invalid()
	}
	if len(parts[2]) > 10 {
		return invalid()
	}
	whole, _ := strconv.ParseInt(parts[2], 10, 64)
	strike := Price(whole * 100)
	var kind OptionType
	switch parts[3] {
	case "CE":
		kind = Call
	case "PE":
		kind = Put
	default:
		return invalid()
	}
	return OptionSymbol{Underlying: parts[0], Expiry: expiry, StrikePrice: strike, OptionType: kind}, nil
}
