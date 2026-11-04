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
		{"known contract", Order{Side: "BUY", Quantity: 75, Request: ContractRequest{ContractID: 101}}, ""},
		{"ATM zero offset", Order{Side: "SELL", Quantity: 75, Request: ContractRequest{OptionType: Call}}, ""},
		{"negative offset", Order{Side: "BUY", Quantity: 75, Request: ContractRequest{OptionType: Put, Offset: -2}}, ""},
		{"invalid side", Order{Side: "SHORT", Quantity: 75, Request: ContractRequest{ContractID: 101}}, "side"},
		{"zero quantity", Order{Side: "BUY", Request: ContractRequest{ContractID: 101}}, "positive"},
		{"negative quantity", Order{Side: "SELL", Quantity: -1, Request: ContractRequest{ContractID: 101}}, "positive"},
		{"negative ID", Order{Side: "BUY", Quantity: 75, Request: ContractRequest{ContractID: -1}}, "positive ID"},
		{"ID and type", Order{Side: "BUY", Quantity: 75, Request: ContractRequest{ContractID: 101, OptionType: Call}}, "positive ID"},
		{"ID and offset", Order{Side: "SELL", Quantity: 75, Request: ContractRequest{ContractID: 101, Offset: 2}}, "positive ID"},
		{"missing contract", Order{Side: "BUY", Quantity: 75}, "option_type"},
		{"invalid option type", Order{Side: "BUY", Quantity: 75, Request: ContractRequest{OptionType: "CE"}}, "option_type"},
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
