package auth

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	Svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group("/auth")
	{
		oauthGroup := authGroup.Group("/oauth/google")
		{
			// 사용자를 구글 로그인 페이지로 보냄
			oauthGroup.GET("", h.Login)
			// 구글 로그인 후 돌아오는 경로
			oauthGroup.GET("/callback", h.Callback)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	// 1. AccessToken, RefreshToken이 다 담긴 User 객체를 반환
	user, err := h.Svc.ProcessGoogleCallback(c.Request.Context(), code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, getFrontendURL()+"/login?error=auth_failed")
		return
	}

	// 환경 변수에서 주소를 가져오고, 없으면 로컬 주소를 기본값으로 사용
	frontendURL := getFrontendURL()
	token := user.AccessToken

	targetURL := fmt.Sprintf("%s/auth/callback?token=%s", frontendURL, token)
	c.Redirect(http.StatusFound, targetURL)
}

func getFrontendURL() string {
	url := os.Getenv("FRONTEND_URL")
	if url == "" {
		// 설정이 없으면 개발 환경(로컬)으로 간주
		return "http://localhost:4173"
	}
	return strings.TrimSuffix(url, "/")
}
