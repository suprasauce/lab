package main

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"optionsbacktest/controller"
	"optionsbacktest/logging"
)

func main() {
	logger := logging.NewLogger(slog.LevelInfo)

	router := gin.New()
	router.Use(gin.Recovery())
	api := router.Group("/api")
	api.POST("/backtests", controller.CreateBacktest)

	if err := router.Run(":8080"); err != nil {
		logger.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
