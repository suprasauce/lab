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

type createStrategyRequest struct {
	Name   string `json:"name" binding:"required"`
	Script string `json:"script" binding:"required"`
}

func CreateStrategy(ctx *gin.Context, repo *repository.Repository, logger *slog.Logger) {
	var headers validation.UserHeaders
	var request createStrategyRequest
	if err := validation.BindAndValidate(ctx, &headers, &request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := service.CreateStrategy(ctx.Request.Context(), repo, headers.UserID, request.Name, request.Script)
	if err != nil {
		logger.ErrorContext(ctx.Request.Context(), "failed to create strategy", "error", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create strategy."})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"id": strconv.FormatInt(id, 10)})
}
