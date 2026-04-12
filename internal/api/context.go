package api

import (
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OwnerID는 gin context에서 인증된 사용자의 ID를 추출합니다.
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
