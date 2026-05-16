package auth

import (
	"context"
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

// sshStatePrefix marks an OAuth state string as belonging to the ssh-gateway
// keyboard-interactive flow; the suffix is the gateway-issued nonce.
const sshStatePrefix = "ssh:"

// sshCallbackHandler is satisfied by *access.SSHService. Duck-typed to avoid
// a back-edge from access to auth.
type sshCallbackHandler interface {
	HandleSSHCallback(ctx context.Context, nonce, userEmail string) error
}

type Handler struct {
	Svc             *Service
	ssh             sshCallbackHandler // nil disables the ssh-gateway callback branch
	frontendBaseURL string
}

func NewHandler(svc *Service, ssh sshCallbackHandler, frontendBaseURL string) *Handler {
	return &Handler{Svc: svc, ssh: ssh, frontendBaseURL: frontendBaseURL}
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group(routeAuthPrefix)
	{
		authGroup.GET(pathMe, h.Me)
		oauthGroup := authGroup.Group(routeOAuthGooglePrefix)
		{
			oauthGroup.GET("", h.Login)
			// 구글 로그인 후 돌아오는 경로
			oauthGroup.GET(pathOAuthCallback, h.Callback)
		}
	}
}

// Login redirects to Google. Accepts an optional `state` query parameter — used
// by the SSH gateway flow with `ssh:<nonce>`.
func (h *Handler) Login(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	url := h.Svc.BuildLoginURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *Handler) Callback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: ErrOAuthCodeRequired.Error()})
		return
	}
	state := c.Query("state")

	if strings.HasPrefix(state, sshStatePrefix) {
		if h.ssh == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ssh handler not configured"})
			return
		}
		nonce := strings.TrimPrefix(state, sshStatePrefix)
		email, err := h.Svc.VerifyGoogleCode(c.Request.Context(), code)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		if err := h.ssh.HandleSSHCallback(c.Request.Context(), nonce, email); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.Redirect(http.StatusFound, h.frontendBaseURL+"/ssh/complete")
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

func authCookieSecure() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv(envAuthCookieSecure)), "false")
}
