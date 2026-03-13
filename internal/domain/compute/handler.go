package compute

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// Handler는 HTTP 요청을 처리합니다.
type Handler struct {
	Svc *Service
}

// NewHandler는 새로운 핸들러를 생성합니다.
func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

// GetFlavors 핸들러 함수
func (h *Handler) GetFlavors(c *gin.Context) {
	flavors, err := h.Svc.GetAvailableFlavors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "사양 조회를 실패했습니다: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, flavors)
}