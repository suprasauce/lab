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
		{"missing trading symbol", Order{Side: "BUY", Quantity: 75}, "tradingsymbol"},
		{"valid BUY", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Side: "BUY", Quantity: 75}, ""},
		{"valid SELL", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Side: "SELL", Quantity: 75}, ""},
		{"invalid side", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Side: "SHORT", Quantity: 75}, "side"},
		{"zero quantity", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Side: "BUY"}, "positive"},
		{"negative quantity", Order{TradingSymbol: "NIFTY-20260127-22650-CE", Side: "SELL", Quantity: -1}, "positive"},
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
