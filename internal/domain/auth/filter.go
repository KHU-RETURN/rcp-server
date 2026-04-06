package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyUserEmail  = "userEmail"
	ContextKeyAuthClaims = "authClaims"
	ContextKeyUser       = "currentUser"
)

var errInvalidAuthorizationHeader = errors.New("invalid authorization header")

func (h *Handler) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization required"})
			return
		}

		claims, err := h.Svc.TokenService.ValidateToken(tokenString)
		if err != nil || claims.Type != "access" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
			return
		}

		user, err := h.Svc.repo.FindByEmail(c.Request.Context(), claims.Email)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to verify user"})
			return
		}
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}

		c.Set(ContextKeyUserEmail, claims.Email)
		c.Set(ContextKeyAuthClaims, claims)
		c.Set(ContextKeyUser, user)
		c.Next()
	}
}

func extractBearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", errInvalidAuthorizationHeader
	}

	return parts[1], nil
}
