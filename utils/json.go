package utils

import (
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/models"
)

type backtestSummary struct {
	TotalTrades int         `json:"total_trades"`
	TotalPnL    json.Number `json:"total_pnl"`
}

type backtestTrade struct {
	Expiry         string              `json:"expiry"`
	Underlying     string              `json:"underlying"`
	Strike         json.Number         `json:"strike"`
	OptionType     models.OptionType   `json:"option_type"`
	Side           models.PositionSide `json:"side"`
	EntryTimestamp string              `json:"entry_timestamp"`
	ExitTimestamp  *string             `json:"exit_timestamp"`
	EntryPrice     json.Number         `json:"entry_price"`
	ExitPrice      *json.Number        `json:"exit_price"`
	Quantity       int64               `json:"quantity"`
	PnL            *json.Number        `json:"pnl"`
}

// MarshalBacktestResult preserves CSV fields, IST timestamps, and exact rupee decimals.
// Open positions have null exit fields and P&L and contribute no realized P&L.
func MarshalBacktestResult(result *engine.BacktestResult) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("backtest result cannot be nil")
	}
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return nil, fmt.Errorf("load result timezone: %w", err)
	}
	output := struct {
		Summary backtestSummary `json:"summary"`
		Trades  []backtestTrade `json:"trades"`
	}{Trades: make([]backtestTrade, 0)}
	totalPnL := new(big.Int)
	for _, cycle := range result.Executions {
		for _, position := range cycle {
			if position == nil || position.Contract == nil {
				return nil, fmt.Errorf("backtest position has no contract")
			}
			trade := backtestTrade{
				Expiry:         position.Contract.Expiry.Format(time.DateOnly),
				Underlying:     result.Underlying,
				Strike:         json.Number(formatPrice(position.Contract.StrikePrice)),
				OptionType:     position.Contract.OptionType,
				Side:           position.Side,
				EntryTimestamp: position.EntryTime.In(location).Format(time.RFC3339),
				EntryPrice:     json.Number(formatPrice(position.EntryPrice)),
				Quantity:       position.Quantity,
			}
			if position.Status == models.TradeClosed {
				exitTime := position.ExitTime.In(location).Format(time.RFC3339)
				exitPrice := json.Number(formatPrice(position.ExitPrice))
				pnl := json.Number(formatPrice(position.PnL))
				trade.ExitTimestamp, trade.ExitPrice, trade.PnL = &exitTime, &exitPrice, &pnl
				totalPnL.Add(totalPnL, big.NewInt(int64(position.PnL)))
			}
			output.Trades = append(output.Trades, trade)
		}
	}
	output.Summary.TotalTrades = len(output.Trades)
	output.Summary.TotalPnL = json.Number(new(big.Rat).SetFrac(totalPnL, big.NewInt(100)).FloatString(2))
	return json.Marshal(output)
}
