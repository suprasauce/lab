package engine

import (
	"time"

	"optionsbacktest/models"
)

// MTMPoint records unrealized P&L observed at a simulation timestamp.
type MTMPoint struct {
	Timestamp time.Time
	PnL       models.Price
}

// Position is reused when the same symbol is reopened within a simulation.
// RealizedPnL accumulates across closes; quantity is signed lots, with zero closed.
type Position struct {
	SimulationID  string
	TradingSymbol string
	Quantity      int64
	AveragePrice  models.Price
	RealizedPnL   models.Price
	MTM           []MTMPoint
	Trades        []models.Trade
	Candle        *models.Candle
}
