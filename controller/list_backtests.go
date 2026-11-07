package controller

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"optionsbacktest/repository"
	"optionsbacktest/service"
	"optionsbacktest/validation"
)

func ListBacktests(ctx *gin.Context, repo *repository.Repository, logger *slog.Logger) {
	var headers validation.UserHeaders
	if err := validation.BindAndValidate(ctx, &headers, nil); err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "A valid X-User-ID header is required."})
		return
	}
	response, err := service.ListBacktests(ctx.Request.Context(), repo, headers.UserID)
	if err != nil {
		logger.ErrorContext(ctx.Request.Context(), "failed to list backtests", "error", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve backtests."})
		return
	}
	ctx.JSON(http.StatusOK, response)
}
