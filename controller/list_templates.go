package controller

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"optionsbacktest/repository"
	"optionsbacktest/service"
)

func ListTemplates(ctx *gin.Context, repo *repository.Repository, logger *slog.Logger) {
	response, err := service.ListTemplates(ctx.Request.Context(), repo)
	if err != nil {
		logger.ErrorContext(ctx.Request.Context(), "failed to list templates", "error", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve templates."})
		return
	}
	ctx.JSON(http.StatusOK, response)
}
