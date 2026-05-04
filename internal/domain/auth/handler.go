package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// sshCallbackHandler is satisfied by ssh.Service. Defined here to avoid an
// import cycle between auth and ssh domains.
type sshCallbackHandler interface {
	HandleSSHCallback(ctx context.Context, nonce, userEmail string) error
}

type Handler struct {
	Svc *Service
	ssh sshCallbackHandler // optional: nil disables the ssh:<nonce> branch
}

func NewHandler(svc *Service, ssh sshCallbackHandler) *Handler {
	return &Handler{Svc: svc, ssh: ssh}
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

	// SSH-flow branch: state begins with "ssh:".
	if strings.HasPrefix(state, "ssh:") {
		if h.ssh == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ssh handler not configured"})
			return
		}
		nonce := strings.TrimPrefix(state, "ssh:")
		email, err := h.Svc.VerifyGoogleCode(c.Request.Context(), code)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		if err := h.ssh.HandleSSHCallback(c.Request.Context(), nonce, email); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html>
<title>RCP SSH</title>
<style>body{font-family:sans-serif;padding:32px}</style>
<h1>로그인 완료</h1>
<p>SSH 터미널로 돌아가세요.</p>`))
		return
	}

	user, err := h.Svc.ProcessGoogleCallback(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "login success", "user": user})
}
