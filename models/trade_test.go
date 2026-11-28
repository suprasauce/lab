package models

import (
	"strings"
	"testing"
)

func TestTradeValidate(t *testing.T) {
	for _, tt := range []struct {
		name  string
		trade Trade
		want  string
	}{
		{"missing trading symbol", Trade{Quantity: 75}, "tradingsymbol"},
		{"valid BUY", Trade{TradingSymbol: "NIFTY-20260127-22650-CE", Quantity: 75}, ""},
		{"valid SELL", Trade{TradingSymbol: "NIFTY-20260127-22650-CE", Quantity: -75}, ""},
		{"zero quantity", Trade{TradingSymbol: "NIFTY-20260127-22650-CE"}, "nonzero"},
		{"negative quantity", Trade{TradingSymbol: "NIFTY-20260127-22650-CE", Quantity: -1}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.trade.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v; want %q", err, tt.want)
			}
		})
	}
}
