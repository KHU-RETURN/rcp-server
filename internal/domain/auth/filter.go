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

	headerAuthorization = "Authorization"
	schemeBearer        = "Bearer"
)

var errInvalidAuthorizationHeader = errors.New("invalid authorization header")

func (h *Handler) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := accessTokenFromRequest(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrAccessTokenNotFound.Error()})
			return
		}

		claims, err := h.Svc.TokenService.ValidateToken(tokenString)
		if err != nil || claims.Type != tokenTypeAccess {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrInvalidAccessToken.Error()})
			return
		}

		user, err := h.Svc.repo.FindByEmail(c.Request.Context(), claims.Email)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{Error: ErrUserLookupFailed.Error()})
			return
		}
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrUserNotFound.Error()})
			return
		}

		c.Set(ContextKeyUserEmail, claims.Email)
		c.Set(ContextKeyAuthClaims, claims)
		c.Set(ContextKeyUser, user)
		c.Next()
	}
}

func accessTokenFromRequest(c *gin.Context) (string, error) {
	header := c.GetHeader(headerAuthorization)
	if strings.TrimSpace(header) != "" {
		return extractBearerToken(header)
	}

	token, err := c.Cookie(cookieAccessToken)
	if err != nil || token == "" {
		return "", ErrAccessTokenNotFound
	}
	return token, nil
}

// refreshTokenFromRequest는 refresh token을 cookie에서만 읽습니다.
// Authorization 헤더는 access 전용이므로 의도적으로 fallback을 두지 않습니다.
func refreshTokenFromRequest(c *gin.Context) (string, error) {
	token, err := c.Cookie(cookieRefreshToken)
	if err != nil || token == "" {
		return "", ErrRefreshTokenNotFound
	}
	return token, nil
}

func extractBearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], schemeBearer) || parts[1] == "" {
		return "", errInvalidAuthorizationHeader
	}

	return parts[1], nil
}
