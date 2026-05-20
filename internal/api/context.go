package api

import (
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
