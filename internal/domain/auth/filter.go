package auth

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ContextKeyUserEmail  = "userEmail"
	ContextKeyAuthClaims = "authClaims"
	ContextKeyUser       = "currentUser"

	headerAuthorization = "Authorization"
	schemeBearer        = "Bearer"
	envAdminEmails      = "RCP_ADMIN_EMAILS"
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

func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get(ContextKeyUser)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrUserNotFound.Error()})
			return
		}

		user, ok := val.(*User)
		if !ok || !IsAdminEmail(user.Email) {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "admin access required"})
			return
		}

		c.Next()
	}
}

func IsAdminEmail(email string) bool {
	target := strings.ToLower(strings.TrimSpace(email))
	if target == "" {
		return false
	}

	for _, item := range strings.Split(os.Getenv(envAdminEmails), ",") {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

func RoleForEmail(email string) string {
	if IsAdminEmail(email) {
		return "admin"
	}
	return "user"
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
