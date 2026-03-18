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
// GetInstanceDetail 핸들러
func (h *Handler) GetInstanceDetail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "인스턴스 ID가 필요합니다."})
		return
	}

	detail, err := h.Svc.GetInstanceDetail(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "상세 조회 실패: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, detail)
}

func (h *Handler) GetInstances(c *gin.Context) {
	instances, err := h.Svc.GetInstances()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, instances)
}

// InitRoutes는 전달받은 RouterGroup에 Compute 관련 엔드포인트들을 등록합니다.
func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
    computeGroup := rg.Group("/compute")
    {
        computeGroup.GET("/flavors", h.GetFlavors)
        computeGroup.GET("/instances", h.GetInstances) // 목록 조회 추가!
        computeGroup.GET("/instances/detail/:id", h.GetInstanceDetail)
    }
}
