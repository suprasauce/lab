package tests

import (
	"log/slog"
	"net/http/httptest"

	"github.com/gin-gonic/gin"

	"optionsbacktest/controller"
	"optionsbacktest/repository"
	"optionsbacktest/worker"
)

// newTestServer registers API routes for an isolated test server.
func newTestServer(repo *repository.Repository, pool *worker.WorkerPool, logger *slog.Logger) *httptest.Server {
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
	api.POST("/strategies", func(ctx *gin.Context) {
		controller.CreateStrategy(ctx, repo, logger)
	})
	api.GET("/strategies", func(ctx *gin.Context) {
		controller.ListStrategies(ctx, repo, logger)
	})
	return httptest.NewServer(router)
}
