package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"optionsbacktest/database"
	"optionsbacktest/engine"
	"optionsbacktest/logging"
	"optionsbacktest/repository"
	"optionsbacktest/scripting"
	"optionsbacktest/utils"
)

func main() {
	strategyPath := flag.String("strategy", "strategies/short_call.star", "Starlark strategy file")
	flag.Parse()
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
	script, err := scripting.Load(*strategyPath)
	if err != nil {
		logger.Error("failed to load strategy", "error", err)
		return
	}
	results, err := script.Run(ctx, engine.NewEngine(marketDataRepository))
	if err != nil {
		logger.Error("backtest failed", "error", err)
		return
	}
	if err := os.MkdirAll("results", 0755); err != nil {
		logger.Error("failed to create results directory", "error", err)
		return
	}
	const outputPath = "results/backtest.csv"
	output, err := os.Create(outputPath) // Truncate the previous run's CSV.
	if err != nil {
		logger.Error("failed to create backtest CSV", "error", err)
		return
	}
	writeErr := utils.WriteBacktestCSV(output, results)
	closeErr := output.Close()
	if writeErr != nil {
		logger.Error("failed to write backtest CSV", "error", writeErr)
		return
	}
	if closeErr != nil {
		logger.Error("failed to close backtest CSV", "error", closeErr)
		return
	}
	logger.Info("backtest completed", "expiry_cycles", len(results.Executions), "csv", outputPath)
}
