package utils

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"testing"

	"optionsbacktest/engine"
)

func TestMarshalBacktestResultMatchesCSV(t *testing.T) {
	result := csvResult()
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
	var csvOutput bytes.Buffer
	if err := WriteBacktestCSV(&csvOutput, result); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&csvOutput).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Trades) != len(rows)-1 {
		t.Fatal("JSON and CSV row counts differ")
	}
	for i, trade := range output.Trades {
		if len(trade) != len(rows[0]) {
			t.Fatal("JSON and CSV field counts differ")
		}
		for column, name := range rows[0] {
			value, exists := trade[name]
			if !exists {
				t.Fatalf("missing CSV field %s", name)
			}
			var actual string
			switch value := value.(type) {
			case string:
				actual = value
			case json.Number:
				actual = value.String()
			case nil:
				actual = ""
			default:
				t.Fatalf("unexpected value type for %s", name)
			}
			if actual != rows[i+1][column] {
				t.Fatalf("row %d field %s: JSON=%q CSV=%q", i, name, actual, rows[i+1][column])
			}
		}
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
