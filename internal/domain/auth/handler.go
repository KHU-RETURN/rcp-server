package auth

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	routeAuthPrefix        = "/auth"
	routeOAuthGooglePrefix = "/oauth/google"

	pathMe            = "/me"
	pathOAuthCallback = "/callback"
	pathLoginError    = "/login?error=auth_failed"
	pathAuthCallback  = "/auth/callback"

	envFrontendURL      = "FRONTEND_URL"
	envAuthCookieSecure = "RCP_AUTH_COOKIE_SECURE"

	cookieAccessToken  = "access_token"
	cookieRefreshToken = "refresh_token"

	defaultFrontendURL = "http://localhost:4173"
)

type Handler struct {
	Svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group(routeAuthPrefix)
	{
		authGroup.GET(pathMe, h.Me)
		oauthGroup := authGroup.Group(routeOAuthGooglePrefix)
		{
			// 사용자를 구글 로그인 페이지로 보냄
			oauthGroup.GET("", h.Login)
			// 구글 로그인 후 돌아오는 경로
			oauthGroup.GET(pathOAuthCallback, h.Callback)
		}
	}
}

// Login: GET /api/v1/auth/oauth/google
func (h *Handler) Login(c *gin.Context) {
	url := h.Svc.GetGoogleLoginURL()
	// 구글 승인 서버로 리다이렉트
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// Callback: GET /api/v1/auth/oauth/google/callback
func (h *Handler) Callback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: ErrOAuthCodeRequired.Error()})
		return
	}

	user, err := h.Svc.ProcessGoogleCallback(c.Request.Context(), code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+pathLoginError)
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)

	c.SetCookie(
		cookieAccessToken,
		user.AccessToken,
		60*60, // 1 hour
		"/",
		"",
		authCookieSecure(),
		true, // HttpOnly
	)

	c.SetCookie(
		cookieRefreshToken,
		user.RefreshToken,
		60*60*24*14, // 14 days
		"/",
		"",
		authCookieSecure(),
		true, // HttpOnly
	)

	c.Redirect(http.StatusFound, getFrontendURL()+pathAuthCallback)
}

// GET /api/v1/auth/me
func (h *Handler) Me(c *gin.Context) {
	token, err := accessTokenFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: ErrAccessTokenNotFound.Error()})
		return
	}

	user, err := h.Svc.GetUserByAccessToken(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: ErrInvalidAccessToken.Error()})
		return
	}

	c.JSON(http.StatusOK, MeResponse{
		ID:    user.ID,
		Name:  user.Name,
		Email: user.Email,
	})
}

func getFrontendURL() string {
	url := os.Getenv(envFrontendURL)
	if url == "" {
		// 설정이 없으면 개발 환경(로컬)으로 간주
		return defaultFrontendURL
	}
	return strings.TrimSuffix(url, "/")
}

func authCookieSecure() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv(envAuthCookieSecure)), "false")
}
