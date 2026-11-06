package controller

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"optionsbacktest/repository"
	"optionsbacktest/service"
	"optionsbacktest/validation"
)

type createBacktestRequest struct {
	Script string `json:"script" binding:"required"`
}

func CreateBacktest(ctx *gin.Context, repo *repository.Repository, logger *slog.Logger) {
	var headers validation.UserHeaders
	var request createBacktestRequest
	if err := validation.BindAndValidate(ctx, &headers, &request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := service.SubmitBacktest(ctx.Request.Context(), repo, headers.UserID, request.Script)
	if err != nil {
		logger.ErrorContext(ctx.Request.Context(), "failed to create backtest", "error", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Backtest job submission failed."})
		return
	}
	ctx.JSON(http.StatusAccepted, gin.H{"id": strconv.FormatInt(id, 10)})
}
