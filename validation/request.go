package validation

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

type UserHeaders struct {
	UserID int64 `header:"X-User-ID" binding:"required,gt=0"`
}

// BindAndValidate binds headers and JSON using the supplied structs' binding tags.
// Pass nil for headers or body when that part of the request is not needed.
func BindAndValidate(ctx *gin.Context, headers, body any) error {
	if headers != nil {
		if err := ctx.ShouldBindHeader(headers); err != nil {
			return fmt.Errorf("invalid request headers: %w", err)
		}
	}
	if body != nil {
		if err := ctx.ShouldBindJSON(body); err != nil {
			return fmt.Errorf("invalid request body: %w", err)
		}
	}
	return nil
}
