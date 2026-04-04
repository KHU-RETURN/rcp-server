package auth

import (
	"net/http"

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
		// 사용자를 구글 로그인 페이지로 보냄
		authGroup.GET("/login", h.Login)
		// 구글 로그인 후 돌아오는 경로
		authGroup.GET("/google/callback", h.Callback)
	}
}

// Login: GET /api/v1/auth/login
func (h *Handler) Login(c *gin.Context) {
	url := h.Svc.GetGoogleLoginURL()
	// 구글 승인 서버로 리다이렉트
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// Callback: GET /api/v1/auth/google/callback
func (h *Handler) Callback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	// 서비스 계층에서 토큰 교환 및 유저 정보 처리
	user, err := h.Svc.ProcessGoogleCallback(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 성공 시 유저 정보 반환 (실제로는 세션/JWT 등을 발급함)
	c.JSON(http.StatusOK, gin.H{
		"message": "login success",
		"user":    user,
	})
}
