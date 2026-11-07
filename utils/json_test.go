package utils

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/models"
)

func backtestResult() *engine.BacktestResult {
	contract := &models.Option{ID: 101, StrikePrice: 2500005, OptionType: models.Call, Expiry: time.Date(2026, 1, 27, 0, 0, 0, 0, time.UTC)}

	return &engine.BacktestResult{Underlying: "NIFTY", Executions: [][]*engine.Position{{
		{Contract: contract, Trade: models.Trade{ContractID: 101, Side: models.Short, Quantity: 75,
			EntryTime: time.Date(2026, 1, 1, 3, 45, 0, 0, time.UTC), EntryPrice: 12705,
			ExitTime: time.Date(2026, 1, 6, 3, 50, 0, 0, time.UTC), ExitPrice: 8010,
			Status: models.TradeClosed, PnL: 352125}},
		{Contract: contract, Trade: models.Trade{ContractID: 101, Side: models.Long, Quantity: 1,
			EntryTime: time.Date(2026, 1, 7, 3, 45, 0, 0, time.UTC), EntryPrice: 29, Status: models.TradeOpen}},
	}}}

}

func TestMarshalBacktestResult(t *testing.T) {
	result := backtestResult()
	payload, err := MarshalBacktestResult(result)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Summary struct {
			TotalTrades int         `json:"total_trades"`
			TotalPnL    json.Number `json:"total_pnl"`
		} `json:"summary"`
		Trades []map[string]any `json:"trades"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		t.Fatal(err)
	}
	if output.Summary.TotalTrades != 2 || output.Summary.TotalPnL.String() != "3521.25" {
		t.Fatalf("incorrect summary: %+v", output.Summary)
	}
	expected := []map[string]any{
		{"expiry": "2026-01-27", "underlying": "NIFTY", "strike": json.Number("25000.05"), "option_type": "CALL", "side": "SHORT", "entry_timestamp": "2026-01-01T09:15:00+05:30", "exit_timestamp": "2026-01-06T09:20:00+05:30", "entry_price": json.Number("127.05"), "exit_price": json.Number("80.10"), "quantity": json.Number("75"), "pnl": json.Number("3521.25")},
		{"expiry": "2026-01-27", "underlying": "NIFTY", "strike": json.Number("25000.05"), "option_type": "CALL", "side": "LONG", "entry_timestamp": "2026-01-07T09:15:00+05:30", "exit_timestamp": nil, "entry_price": json.Number("0.29"), "exit_price": nil, "quantity": json.Number("1"), "pnl": nil},
	}
	if !reflect.DeepEqual(output.Trades, expected) {
		t.Fatalf("trades=%v; want %v", output.Trades, expected)
	}
}
func TestMarshalBacktestResultEmpty(t *testing.T) {
	payload, err := MarshalBacktestResult(&engine.BacktestResult{})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"summary":{"total_trades":0,"total_pnl":0.00},"trades":[]}` {
		t.Fatalf("unexpected empty result: %s", payload)
	}
}

func TestMarshalBacktestResultRejectsMissingContract(t *testing.T) {
	for _, result := range []*engine.BacktestResult{
		nil,
		{Executions: [][]*engine.Position{{nil}}},
		{Executions: [][]*engine.Position{{{}}}},
	} {
		if _, err := MarshalBacktestResult(result); err == nil {
			t.Fatal("expected invalid result error")
		}
	}
}
