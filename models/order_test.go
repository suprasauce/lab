package models

import (
	"strings"
	"testing"
)

func TestOrderValidate(t *testing.T) {
	for _, tt := range []struct {
		name  string
		order Order
		want  string
	}{
		{"missing trading symbol", Order{Quantity: 75}, "tradingsymbol"},
		{"valid BUY", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Quantity: 75}, ""},
		{"valid SELL", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Quantity: -75}, ""},
		{"zero quantity", Order{TradingSymbol: "NIFTY-20260127-22650-CE"}, "nonzero"},
		{"negative quantity", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Quantity: -1}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.order.Validate()
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
