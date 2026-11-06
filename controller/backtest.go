package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateBacktest(ctx *gin.Context) {
	// TODO: Parse and validate the request, call service.SubmitBacktest, and return 202 with the backtest ID.
	ctx.Status(http.StatusNotImplemented)
}
