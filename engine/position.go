package engine

import "optionsbacktest/models"

// Position is reused when the same symbol is reopened within a simulation.
// RealizedPnL accumulates across closes; zero quantity means closed.
type Position struct {
	SimulationID  string
	TradingSymbol string
	Quantity      int64
	AveragePrice  models.Price
	RealizedPnL   models.Price
	Trades        []models.Trade
	Candle        *models.Candle
}
