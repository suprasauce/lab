package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"optionsbacktest/controller"
	"optionsbacktest/database"
	"optionsbacktest/logging"
	"optionsbacktest/repository"
)

func main() {
	logger := logging.NewLogger(slog.LevelInfo)
	db, err := database.NewPostgres(context.Background())
	if err != nil {
		logger.Error("failed to initialize PostgreSQL", "component", "database")
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("failed to close database pool", "component", "database")
		}
	}()

	repo := repository.NewRepository(db, logger)

	router := gin.New()
	router.Use(gin.Recovery())
	api := router.Group("/api")
	api.POST("/backtests", func(ctx *gin.Context) {
		controller.CreateBacktest(ctx, repo, logger)
	})

	if err := router.Run(":8080"); err != nil {
		logger.Error("HTTP server failed", "error", err)
		return
	}
}
