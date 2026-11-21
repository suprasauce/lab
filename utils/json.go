package utils

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"time"

	"optionsbacktest/engine"
	"optionsbacktest/models"
)

type backtestSummary struct {
	TotalTrades int         `json:"total_trades"`
	TotalPnL    json.Number `json:"total_pnl"`
}

type backtestTrade struct {
	ID            string      `json:"id"`
	SimulationID  string      `json:"simulation_id"`
	TradingSymbol string      `json:"tradingsymbol"`
	Timestamp     string      `json:"timestamp"`
	Price         json.Number `json:"price"`
	Quantity      int64       `json:"quantity"`
}

type backtestPosition struct {
	SimulationID  string            `json:"simulation_id"`
	TradingSymbol string            `json:"tradingsymbol"`
	Quantity      int64             `json:"quantity"`
	AveragePrice  json.Number       `json:"average_price"`
	RealizedPnL   json.Number       `json:"realized_pnl"`
	MTM           []models.MTMPoint `json:"mtm"`
}

// MarshalBacktestResult serializes positions and fills without pairing trades.
// Prices and realized P&L are in rupees with exact paise precision.
func MarshalBacktestResult(result *engine.BacktestResult) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("backtest result cannot be nil")
	}
	location, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return nil, fmt.Errorf("load result timezone: %w", err)
	}
	output := struct {
		Summary   backtestSummary    `json:"summary"`
		Positions []backtestPosition `json:"positions"`
		Trades    []backtestTrade    `json:"trades"`
	}{Positions: make([]backtestPosition, 0), Trades: make([]backtestTrade, 0)}
	totalPnL := new(big.Int)
	for _, simulation := range result.Executions {
		if simulation == nil {
			return nil, fmt.Errorf("backtest execution is nil")
		}
		// Position histories are grouped by symbol internally; order fills by timestamp for display.
		fills := make([]models.Trade, 0)
		for _, position := range simulation.Positions {
			if position == nil {
				return nil, fmt.Errorf("backtest position is nil")
			}
			mtm := make([]models.MTMPoint, 0, len(position.MTM))
			for _, point := range position.MTM {
				mtm = append(mtm, models.MTMPoint{
					Timestamp: point.Timestamp.In(location),
					PnL:       point.PnL,
				})
			}
			output.Positions = append(output.Positions, backtestPosition{
				SimulationID: position.SimulationID, TradingSymbol: position.TradingSymbol,
				Quantity: position.Quantity, AveragePrice: json.Number(formatPrice(position.AveragePrice)),
				RealizedPnL: json.Number(formatPrice(position.RealizedPnL)),
				MTM:         mtm,
			})
			totalPnL.Add(totalPnL, big.NewInt(int64(position.RealizedPnL)))
			fills = append(fills, position.Trades...)
		}
		sort.SliceStable(fills, func(i, j int) bool { return fills[i].Timestamp.Before(fills[j].Timestamp) })
		for _, fill := range fills {
			output.Trades = append(output.Trades, backtestTrade{
				ID: fill.ID, SimulationID: fill.SimulationID, TradingSymbol: fill.TradingSymbol,
				Timestamp: fill.Timestamp.In(location).Format(time.RFC3339),
				Price:     json.Number(formatPrice(fill.Price)), Quantity: fill.Quantity,
			})
		}
	}
	output.Summary.TotalTrades = len(output.Trades)
	output.Summary.TotalPnL = json.Number(new(big.Rat).SetFrac(totalPnL, big.NewInt(100)).FloatString(2))
	return json.Marshal(output)
}

func formatPrice(price models.Price) string { return big.NewRat(int64(price), 100).FloatString(2) }
