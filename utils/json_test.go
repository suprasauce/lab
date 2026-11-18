package utils

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/models"
)

func backtestResult() *engine.BacktestResult {
	return &engine.BacktestResult{Executions: []*engine.Execution{{ID: "1", Positions: []*engine.Position{
		{SimulationID: "1", TradingSymbol: "NIFTY-20260127-25000-CE", Quantity: 1, AveragePrice: 29, RealizedPnL: 352125, Trades: []models.Trade{
			{ID: "1/1", SimulationID: "1", TradingSymbol: "NIFTY-20260127-25000-CE", Quantity: -75, Timestamp: time.Date(2026, 1, 1, 3, 45, 0, 0, time.UTC), Price: 12705},
			{ID: "1/2", SimulationID: "1", TradingSymbol: "NIFTY-20260127-25000-CE", Quantity: 75, Timestamp: time.Date(2026, 1, 6, 3, 50, 0, 0, time.UTC), Price: 8010},
			{ID: "1/3", SimulationID: "1", TradingSymbol: "NIFTY-20260127-25000-CE", Quantity: 1, Timestamp: time.Date(2026, 1, 7, 3, 45, 0, 0, time.UTC), Price: 29},
		}},
	}}}}
}

func TestMarshalBacktestResult(t *testing.T) {
	payload, err := MarshalBacktestResult(backtestResult())
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Summary struct {
			TotalTrades int         `json:"total_trades"`
			TotalPnL    json.Number `json:"total_pnl"`
		} `json:"summary"`
		Positions []map[string]any `json:"positions"`
		Trades    []map[string]any `json:"trades"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		t.Fatal(err)
	}
	if output.Summary.TotalTrades != 3 || output.Summary.TotalPnL.String() != "3521.25" || len(output.Positions) != 1 {
		t.Fatalf("incorrect result: %s", payload)
	}
	position := output.Positions[0]
	if len(position) != 5 || position["simulation_id"] != "1" || position["quantity"] != json.Number("1") || position["average_price"] != json.Number("0.29") || position["realized_pnl"] != json.Number("3521.25") {
		t.Fatalf("incorrect position: %v", position)
	}
	first := output.Trades[0]
	if len(first) != 6 || first["id"] != "1/1" || first["simulation_id"] != "1" || first["tradingsymbol"] != "NIFTY-20260127-25000-CE" || first["timestamp"] != "2026-01-01T09:15:00+05:30" || first["price"] != json.Number("127.05") || first["quantity"] != json.Number("-75") {
		t.Fatalf("incorrect fill: %v", first)
	}
}

func TestMarshalBacktestResultEmpty(t *testing.T) {
	payload, err := MarshalBacktestResult(&engine.BacktestResult{})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"summary":{"total_trades":0,"total_pnl":0.00},"positions":[],"trades":[]}` {
		t.Fatalf("unexpected empty result: %s", payload)
	}
}

func TestMarshalBacktestResultRejectsNil(t *testing.T) {
	for _, result := range []*engine.BacktestResult{nil, {Executions: []*engine.Execution{nil}}, {Executions: []*engine.Execution{{Positions: []*engine.Position{nil}}}}} {
		if _, err := MarshalBacktestResult(result); err == nil {
			t.Fatal("expected invalid result error")
		}
	}
}

func TestResultsSumPositionPnLWithoutPairingTrades(t *testing.T) {
	result := backtestResult()
	// A second simulation may use the same symbol and local order sequence.
	result.Executions = append(result.Executions, &engine.Execution{ID: "2", Positions: []*engine.Position{
		{SimulationID: "2", TradingSymbol: "NIFTY-20260127-25000-CE", Quantity: 0, RealizedPnL: -10000, Trades: []models.Trade{
			{ID: "2/1", SimulationID: "2", TradingSymbol: "NIFTY-20260127-25000-CE", Quantity: 75, Price: 100},
		}},
	}})
	payload, err := MarshalBacktestResult(result)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Summary struct {
			TotalTrades int     `json:"total_trades"`
			TotalPnL    float64 `json:"total_pnl"`
		} `json:"summary"`
		Positions []struct {
			SimulationID string `json:"simulation_id"`
		} `json:"positions"`
	}
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Summary.TotalTrades != 4 || output.Summary.TotalPnL != 3421.25 || len(output.Positions) != 2 || output.Positions[1].SimulationID != "2" {
		t.Fatalf("incorrect totals or identity: %s", payload)
	}
}
