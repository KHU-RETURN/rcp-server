package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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
	authGroup := rg.Group("/auth")
	{
		oauthGroup := authGroup.Group("/oauth/google")
		{
			oauthGroup.GET("", h.Login)
			oauthGroup.GET("/callback", h.Callback)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "login success", "user": user})
}
