package auth

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	routeAuthPrefix        = "/auth"
	routeOAuthGooglePrefix = "/oauth/google"

	pathMe            = "/me"
	pathRefresh       = "/refresh"
	pathLogout        = "/logout"
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
		authGroup.POST(pathRefresh, h.Refresh)
		authGroup.POST(pathLogout, h.Logout)
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

	setAuthCookie(c, cookieAccessToken, user.AccessToken, int(accessTokenTTL.Seconds()))
	setAuthCookie(c, cookieRefreshToken, user.RefreshToken, int(refreshTokenTTL.Seconds()))

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
		ID:          user.ID,
		Name:        user.Name,
		Email:       user.Email,
		AccessToken: token,
	})
}

// Refresh: POST /api/v1/auth/refresh
// refresh_token 쿠키를 검증하고 access/refresh 토큰을 회전 발급합니다.
// 이전 refresh token은 서버측 jti 회전으로 즉시 무효화됩니다.
func (h *Handler) Refresh(c *gin.Context) {
	refreshToken, err := refreshTokenFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: ErrRefreshTokenNotFound.Error()})
		return
	}

	tokens, err := h.Svc.RefreshAccessToken(c.Request.Context(), refreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: ErrInvalidRefreshToken.Error()})
		return
	}

	expiresIn := int(time.Until(tokens.AccessExpiry).Seconds())

	setAuthCookie(c, cookieAccessToken, tokens.AccessToken, expiresIn)
	setAuthCookie(c, cookieRefreshToken, tokens.RefreshToken, int(refreshTokenTTL.Seconds()))

	c.JSON(http.StatusOK, RefreshResponse{
		AccessToken: tokens.AccessToken,
		ExpiresIn:   expiresIn,
	})
}

// Logout: POST /api/v1/auth/logout
// 서버측 refresh jti를 무효화하고 클라이언트 쿠키를 만료시킵니다.
// 토큰이 없거나 유효하지 않아도 항상 204(쿠키 만료는 보장).
func (h *Handler) Logout(c *gin.Context) {
	refreshToken, _ := c.Cookie(cookieRefreshToken)
	if _, err := h.Svc.Logout(c.Request.Context(), refreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: ErrUserLookupFailed.Error()})
		return
	}

	setAuthCookie(c, cookieAccessToken, "", -1)
	setAuthCookie(c, cookieRefreshToken, "", -1)
	c.Status(http.StatusNoContent)
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

// setAuthCookie는 auth 쿠키의 공통 속성(SameSite=Lax, HttpOnly, Secure, path=/)을 일괄 적용합니다.
// 만료 시키려면 maxAge=-1, value=""로 호출.
func setAuthCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, "/", "", authCookieSecure(), true)
}
