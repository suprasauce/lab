package main

import (
	"context"
	"log/slog"
	"os"

	"optionsbacktest/database"
	"optionsbacktest/engine"
	"optionsbacktest/logging"
	"optionsbacktest/repository"
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

	// Strategy execution will be wired here when the scripting layer is available.
	_ = engine.NewEngine(marketDataRepository)
}
