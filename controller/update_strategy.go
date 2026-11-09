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

func UpdateStrategy(ctx *gin.Context, repo *repository.Repository, logger *slog.Logger) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "A valid strategy ID is required."})
		return
	}
	var headers validation.UserHeaders
	var request createStrategyRequest
	if err := validation.BindAndValidate(ctx, &headers, &request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updatedID, err := service.UpdateStrategy(ctx.Request.Context(), repo, id, headers.UserID, request.Name, request.Script)
	if errors.Is(err, sql.ErrNoRows) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Strategy not found."})
		return
	}
	if err != nil {
		logger.ErrorContext(ctx.Request.Context(), "failed to update strategy", "strategy_id", id, "error", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update strategy."})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"id": strconv.FormatInt(updatedID, 10)})
}
