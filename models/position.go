package models

import (
	"encoding/json"
	"math/big"
	"time"
)

type PositionSide string

const (
	Long  PositionSide = "LONG"
	Short PositionSide = "SHORT"
)

// PnLPoint records P&L observed at a simulation timestamp.
// PnL is stored in paise; JSON represents it in rupees.
type PnLPoint struct {
	Timestamp time.Time
	PnL       Price
}

func (point PnLPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Timestamp string      `json:"timestamp"`
		PnL       json.Number `json:"pnl"`
	}{
		Timestamp: point.Timestamp.Format(time.RFC3339),
		PnL:       json.Number(big.NewRat(int64(point.PnL), 100).FloatString(2)),
	})
}

// Position is reused when the same symbol is reopened within a simulation.
// RealizedPnL accumulates across closes; quantity is signed lots for options or units
// for underlyings, with zero closed.
type Position struct {
	SimulationID  string
	TradingSymbol string
	Quantity      int64
	AveragePrice  Price
	RealizedPnL   Price
	UnrealizedPnL Price
	TotalPnL      []PnLPoint
	Trades        []Trade
	Candle        *Candle
}
