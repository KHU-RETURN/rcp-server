package auth

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
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

	envFrontendURL                  = "FRONTEND_URL"
	envAllowedFrontendOriginPattern = "RCP_ALLOWED_FRONTEND_ORIGIN_PATTERN"
	envAuthCookieSecure             = "RCP_AUTH_COOKIE_SECURE"

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
	redirectOrigin := ""
	if rawRedirectOrigin := c.Query("redirect_origin"); rawRedirectOrigin != "" {
		allowedOrigin, ok := allowedFrontendOrigin(rawRedirectOrigin)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid redirect_origin"})
			return
		}
		redirectOrigin = allowedOrigin
	}

	url := h.Svc.GetGoogleLoginURL(redirectOrigin)
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

	frontendURL := frontendURLFromState(c.Query("state"))
	user, err := h.Svc.ProcessGoogleCallback(c.Request.Context(), code)
	if err != nil {
		c.SetSameSite(http.SameSiteLaxMode)
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+pathLoginError)
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

	c.Redirect(http.StatusFound, frontendURL+pathAuthCallback)
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

func getFrontendURL() string {
	url := os.Getenv(envFrontendURL)
	if url == "" {
		// 설정이 없으면 개발 환경(로컬)으로 간주
		return defaultFrontendURL
	}
	return strings.TrimSuffix(url, "/")
}

func frontendURLFromState(raw string) string {
	if raw == "" {
		return getFrontendURL()
	}

	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return getFrontendURL()
	}

	var state oauthState
	if err := json.Unmarshal(b, &state); err != nil {
		return getFrontendURL()
	}

	if state.RedirectOrigin == "" {
		return getFrontendURL()
	}

	frontendURL, ok := allowedFrontendOrigin(state.RedirectOrigin)
	if !ok {
		return getFrontendURL()
	}

	return frontendURL
}

func allowedFrontendOrigin(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}

	host := strings.ToLower(u.Host)
	if !allowedFrontendScheme(u.Scheme, host) {
		return "", false
	}

	origin := u.Scheme + "://" + host
	if frontendURL, ok := configuredFrontendOrigin(); ok && origin == frontendURL {
		return frontendURL, true
	}

	pattern := strings.TrimSpace(os.Getenv(envAllowedFrontendOriginPattern))
	if pattern == "" {
		return "", false
	}

	matched, err := regexp.MatchString(pattern, host)
	if err != nil {
		return "", false
	}
	if matched {
		return origin, true
	}

	return "", false
}

func configuredFrontendOrigin() (string, bool) {
	frontendURL := getFrontendURL()
	u, err := url.Parse(frontendURL)
	if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}

	host := strings.ToLower(u.Host)
	if !allowedFrontendScheme(u.Scheme, host) {
		return "", false
	}

	return u.Scheme + "://" + host, true
}

func allowedFrontendScheme(scheme, host string) bool {
	if scheme == "https" {
		return true
	}
	return scheme == "http" && isLocalhost(host)
}

func isLocalhost(host string) bool {
	hostname := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		hostname = parsedHost
	}

	switch strings.Trim(hostname, "[]") {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func authCookieSecure() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv(envAuthCookieSecure)), "false")
}
