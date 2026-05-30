package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
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

	envFrontendURL                  = "FRONTEND_URL"
	envFrontendBaseURL              = "RCP_FRONTEND_BASE_URL"
	envAllowedFrontendOriginPattern = "RCP_ALLOWED_FRONTEND_ORIGIN_PATTERN"
	envAuthCookieSameSite           = "RCP_AUTH_COOKIE_SAMESITE"
	envAuthCookieSecure             = "RCP_AUTH_COOKIE_SECURE"

	cookieAccessToken  = "access_token"
	cookieRefreshToken = "refresh_token"

	defaultFrontendURL = "http://localhost:4173"
)

// sshStatePrefix marks an OAuth state string as belonging to the ssh-gateway
// keyboard-interactive flow; the suffix is the gateway-issued nonce.
const sshStatePrefix = "ssh:"

// sshCallbackHandler is satisfied by *access.SSHService. Duck-typed to avoid
// a back-edge from access to auth.
type sshCallbackHandler interface {
	HandleSSHCallback(ctx context.Context, nonce, code, userEmail string) error
}

type Handler struct {
	Svc              *Service
	ssh              sshCallbackHandler // nil disables the ssh-gateway callback branch
	frontendBaseURL  string
	verifyGoogleCode func(context.Context, string) (string, error)
}

func NewHandler(svc *Service, ssh sshCallbackHandler, frontendBaseURL string) *Handler {
	return &Handler{
		Svc:              svc,
		ssh:              ssh,
		frontendBaseURL:  strings.TrimRight(strings.TrimSpace(frontendBaseURL), "/"),
		verifyGoogleCode: svc.VerifyGoogleCode,
	}
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group(routeAuthPrefix)
	{
		authGroup.GET(pathMe, h.Me)
		authGroup.POST(pathRefresh, h.Refresh)
		authGroup.POST(pathLogout, h.Logout)
		oauthGroup := authGroup.Group(routeOAuthGooglePrefix)
		{
			oauthGroup.GET("", h.Login)
			// 구글 로그인 후 돌아오는 경로
			oauthGroup.GET(pathOAuthCallback, h.Callback)
		}
	}
}

// Login redirects to Google. An ssh:<nonce>:<code> state is reserved for the
// ssh-gateway keyboard-interactive flow; normal web login uses structured state.
func (h *Handler) Login(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	if strings.HasPrefix(state, sshStatePrefix) {
		if _, ok := parseSSHState(state); !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid ssh state"})
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, h.Svc.BuildLoginURL(state))
		return
	}

	redirectOrigin := ""
	if rawRedirectOrigin := c.Query("redirect_origin"); rawRedirectOrigin != "" {
		allowedOrigin, ok := h.allowedFrontendOrigin(rawRedirectOrigin)
		if !ok {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid redirect_origin"})
			return
		}
		redirectOrigin = allowedOrigin
	}

	c.Redirect(http.StatusTemporaryRedirect, h.Svc.GetGoogleLoginURL(redirectOrigin))
}

func (h *Handler) Callback(c *gin.Context) {
	state := c.Query("state")
	if strings.HasPrefix(state, sshStatePrefix) {
		h.handleSSHCallback(c, state)
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: ErrOAuthCodeRequired.Error()})
		return
	}

	frontendURL := h.frontendURLFromState(state)
	user, err := h.Svc.ProcessGoogleCallback(c.Request.Context(), code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+pathLoginError)
		return
	}

	setAuthCookie(c, cookieAccessToken, user.AccessToken, int(accessTokenTTL.Seconds()))
	setAuthCookie(c, cookieRefreshToken, user.RefreshToken, int(refreshTokenTTL.Seconds()))

	c.Redirect(http.StatusFound, frontendURL+pathAuthCallback)
}

func (h *Handler) handleSSHCallback(c *gin.Context, state string) {
	code := c.Query("code")
	sshState, ok := parseSSHState(state)
	if code == "" || !ok || h.ssh == nil {
		c.Redirect(http.StatusFound, h.sshCompleteURL("failed"))
		return
	}

	email, err := h.verifyGoogleCode(c.Request.Context(), code)
	if err != nil {
		c.Redirect(http.StatusFound, h.sshCompleteURL("failed"))
		return
	}
	if err := h.ssh.HandleSSHCallback(c.Request.Context(), sshState.nonce, sshState.code, email); err != nil {
		c.Redirect(http.StatusFound, h.sshCompleteURL("failed"))
		return
	}
	c.Redirect(http.StatusFound, h.sshCompleteURL(""))
}

type parsedSSHState struct {
	nonce string
	code  string
}

func parseSSHState(raw string) (parsedSSHState, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(raw), sshStatePrefix)
	if !ok {
		return parsedSSHState{}, false
	}
	nonce, code, ok := strings.Cut(rest, ":")
	if !ok || nonce == "" || !validSSHCode(code) {
		return parsedSSHState{}, false
	}
	return parsedSSHState{nonce: nonce, code: code}, true
}

func validSSHCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (h *Handler) sshCompleteURL(status string) string {
	u := h.defaultFrontendURL() + "/ssh/complete"
	if status == "" {
		return u
	}
	return u + "?status=" + url.QueryEscape(status)
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
// 쿠키 만료는 서버측 jti 무효화 성공/실패와 무관하게 항상 보장.
func (h *Handler) Logout(c *gin.Context) {
	refreshToken, _ := c.Cookie(cookieRefreshToken)

	setAuthCookie(c, cookieAccessToken, "", -1)
	setAuthCookie(c, cookieRefreshToken, "", -1)

	if _, err := h.Svc.Logout(c.Request.Context(), refreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: ErrUserLookupFailed.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) defaultFrontendURL() string {
	if h.frontendBaseURL != "" {
		return h.frontendBaseURL
	}
	return getFrontendURL()
}

func (h *Handler) frontendURLFromState(raw string) string {
	return frontendURLFromStateWith(raw, h.defaultFrontendURL(), h.allowedFrontendOrigin)
}

func (h *Handler) allowedFrontendOrigin(raw string) (string, bool) {
	return allowedFrontendOriginWith(raw, h.defaultFrontendURL())
}

func getFrontendURL() string {
	if url := strings.TrimSpace(os.Getenv(envFrontendURL)); url != "" {
		return strings.TrimSuffix(url, "/")
	}
	if url := strings.TrimSpace(os.Getenv(envFrontendBaseURL)); url != "" {
		return strings.TrimSuffix(url, "/")
	}
	return defaultFrontendURL
}

func frontendURLFromStateWith(raw, fallback string, allowOrigin func(string) (string, bool)) string {
	if raw == "" {
		return fallback
	}

	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return fallback
	}

	var state oauthState
	if err := json.Unmarshal(b, &state); err != nil {
		return fallback
	}

	if state.RedirectOrigin == "" {
		return fallback
	}

	frontendURL, ok := allowOrigin(state.RedirectOrigin)
	if !ok {
		return fallback
	}

	return frontendURL
}

func allowedFrontendOrigin(raw string) (string, bool) {
	return allowedFrontendOriginWith(raw, getFrontendURL())
}

func allowedFrontendOriginWith(raw, configured string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}

	host := strings.ToLower(u.Host)
	if !allowedFrontendScheme(u.Scheme, host) {
		return "", false
	}

	origin := u.Scheme + "://" + host
	if frontendURL, ok := configuredFrontendOriginFromURL(configured); ok && origin == frontendURL {
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

func configuredFrontendOriginFromURL(frontendURL string) (string, bool) {
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

// setAuthCookie는 auth 쿠키의 공통 속성(SameSite, HttpOnly, Secure, path=/)을 일괄 적용합니다.
// 만료 시키려면 maxAge=-1, value=""로 호출.
func setAuthCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(authCookieSameSite())
	c.SetCookie(name, value, maxAge, "/", "", authCookieSecure(), true)
}

func authCookieSameSite() http.SameSite {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAuthCookieSameSite))) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}
