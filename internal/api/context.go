package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
)

func OwnerID(c *gin.Context) (uuid.UUID, bool) {
	val, exists := c.Get(auth.ContextKeyUser)
	if !exists {
		return uuid.UUID{}, false
	}
	u, ok := val.(*auth.User)
	if !ok {
		return uuid.UUID{}, false
	}
	return u.ID, true
}

// MustOwnerID returns the authenticated owner ID, or aborts with 401 and
// reports false. AuthRequired guarantees the user is set, so the false path
// only fires on misconfiguration (a protected route without the middleware).
func MustOwnerID(c *gin.Context) (uuid.UUID, bool) {
	id, ok := OwnerID(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return uuid.UUID{}, false
	}
	return id, true
}
