package controller

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"optionsbacktest/repository"
	"optionsbacktest/service"
	"optionsbacktest/validation"
)

func GetBacktest(ctx *gin.Context, repo *repository.Repository, logger *slog.Logger) {
	var headers validation.UserHeaders
	if err := validation.BindAndValidate(ctx, &headers, nil); err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "A valid X-User-ID header is required."})
		return
	}
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Backtest not found."})
		return
	}
	response, err := service.GetBacktest(ctx.Request.Context(), repo, id, headers.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Backtest not found."})
		return
	}
	if err != nil {
		logger.ErrorContext(ctx.Request.Context(), "failed to retrieve backtest", "backtest_id", id, "error", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve backtest."})
		return
	}
	ctx.JSON(http.StatusOK, response)
}
