package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"optionsbacktest/database"
	"optionsbacktest/engine"
	"optionsbacktest/logging"
	"optionsbacktest/models"
	"optionsbacktest/repository"
	"optionsbacktest/selector"
)

func main() {
	logger := logging.NewLogger(slog.LevelInfo)

	db, err := database.NewPostgres(context.Background())
	if err != nil {
		// Driver errors may contain connection details; do not log them verbatim.
		logger.Error(err.Error(), "component", "database")
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("failed to close database pool", "component", "database")
		}
	}()

	marketDataRepository := repository.NewMarketDataRepository(db, logger)

	ctx := context.Background()
	underlyings, err := marketDataRepository.GetAllUnderlyings(ctx)
	if err != nil {
		logger.Error("failed to fetch underlyings", "error", err)
		return
	}
	var underlyingID int64
	for _, underlying := range underlyings {
		if underlying.Symbol == "NIFTY" {
			underlyingID = underlying.ID
			break
		}
	}
	if underlyingID == 0 {
		logger.Error("underlying not found", "symbol", "NIFTY")
		return
	}

	leg := models.OptionLeg{
		OptionType:     models.Call,
		PositionSide:   models.Short,
		Quantity:       1,
		EntryDTE:       21,
		EntryHour:      9,
		EntryMinute:    30,
		StrikeSelector: selector.NewOffsetSelector(2),
	}
	strategy := models.Strategy{
		Type:         models.StrategyOptions,
		UnderlyingID: underlyingID,
		StartDate:    time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
		Legs:         []models.OptionLeg{leg},
	}
	backtestEngine := engine.NewEngine(marketDataRepository)
	if err := backtestEngine.Backtest(ctx, &strategy); err != nil {
		logger.Error("backtest failed", "error", err)
		return
	}
	logger.Info("backtest completed successfully")
}
