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
	"optionsbacktest/worker"
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
	pool := worker.NewWorkerPool(repo, logger)
	pool.Start()

	router := gin.New()
	router.Use(gin.Recovery())
	api := router.Group("/api")
	api.GET("/backtests", func(ctx *gin.Context) {
		controller.ListBacktests(ctx, repo, logger)
	})
	api.GET("/backtests/:id", func(ctx *gin.Context) {
		controller.GetBacktest(ctx, repo, logger)
	})
	api.POST("/backtests", func(ctx *gin.Context) {
		controller.CreateBacktest(ctx, repo, pool, logger)
	})

	if err := router.Run(":8080"); err != nil {
		logger.Error("HTTP server failed", "error", err)
		return
	}
}
