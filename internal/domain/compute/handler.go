package compute

import (
	"errors"
	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
)

// Handler는 HTTP 요청을 처리합니다.
type Handler struct {
	Svc *Service
}

// NewHandler는 새로운 핸들러를 생성합니다.
func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

var ErrInstanceNotFound = errors.New("instance not found")

// GetFlavors godoc
// @Summary List compute flavors
// @Description Returns the currently available flavor catalog.
// @Tags compute
// @Produce json
// @Success 200 {array} FlavorResponse
// @Failure 500 {object} api.ErrorResponse
// @Router /api/v1/compute/flavors [get]
// @Router /api/v1/compute/flavors/all [get]
func (h *Handler) GetFlavors(c *gin.Context) {
	flavors, err := h.Svc.GetFlavors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "사양 조회를 실패했습니다: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, flavors)
}

// InitRoutes는 전달받은 RouterGroup에 Compute 관련 엔드포인트들을 등록합니다.
func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	computeGroup := rg.Group("/compute") // /api/v1/compute
	{
		// 기존 클라이언트 호환을 위해 /flavors 경로를 유지합니다.
		computeGroup.GET("/flavors", h.GetFlavors)
		// 전체 flavors 조회 별칭
		computeGroup.GET("/flavors/all", h.GetFlavors)
		// 남은 자원량 기반 가용 flavors 조회
		computeGroup.GET("/flavors/available", h.GetAvailableFlavors)
		// 인스턴스 서버 생성 엔드포인트
		computeGroup.POST("/instances", h.CreateServer)
		// 인스턴스 서버 삭제 엔드포인트
		computeGroup.DELETE("/instances/:id", h.DeleteServer)
	}
}

// GetAvailableFlavors godoc
// @Summary List flavors with remaining capacity
// @Description Calculates how many instances can still be created for each flavor based on quota.
// @Tags compute
// @Produce json
// @Success 200 {array} AvailableFlavorResponse
// @Failure 500 {object} api.ErrorResponse
// @Router /api/v1/compute/flavors/available [get]
func (h *Handler) GetAvailableFlavors(c *gin.Context) {
	// 인프라 레이어를 직접 안 부르고 Service(또는 Repo)를 거칩니다.
	client, err := h.Svc.GetComputeClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "Cloud connection failed"})
		return
	}

	projectID := os.Getenv("OS_PROJECT_ID")

	flavors, err := h.Svc.GetAvailableFlavorsWithLimit(client, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, flavors)
}

// CreateServer godoc
// @Summary Create a compute instance
// @Description Creates a new OpenStack server instance.
// @Tags compute
// @Accept json
// @Produce json
// @Param request body CreateInstanceRequest true "Instance creation request"
// @Success 201 {object} CreateInstanceResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Router /api/v1/compute/instances [post]
func (h *Handler) CreateServer(c *gin.Context) {
	var req CreateInstanceRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		return
	}

	opts, err := buildCreateServerOpts(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrCreateInstanceNameRequired),
			errors.Is(err, ErrCreateInstanceImageRequired),
			errors.Is(err, ErrCreateInstanceFlavorRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		}
		return
	}

	client, err := h.Svc.GetComputeClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "Failed to connect to cloud"})
		return
	}

	server, err := h.Svc.CreateInstance(client, opts)
	if err != nil {
		switch {
		case errors.Is(err, ErrCreateInstanceNameRequired),
			errors.Is(err, ErrCreateInstanceImageRequired),
			errors.Is(err, ErrCreateInstanceFlavorRequired):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, server)
}

func (h *Handler) DeleteServer(c *gin.Context) {
	// URL 파라미터에서 ID 추출
	serverID := c.Param("id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server ID is required"})
		return
	}

	client, err := h.Svc.GetComputeClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cloud connection failed"})
		return
	}

	// 삭제 서비스 호출
	if err := h.Svc.DeleteInstance(client, serverID); err != nil {
		// h.Svc.DeleteInstance에서 ErrInstanceNotFound를 리턴한다고 가정
		if errors.Is(err, ErrInstanceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "해당 인스턴스를 찾을 수 없습니다.",
				"code":  "INSTANCE_NOT_FOUND",
			})
		} else {
			// 예상치 못한 시스템 에러
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "인스턴스 삭제 중 서버 오류가 발생했습니다.",
				"detail": err.Error(),
			})
		}
		return
	}

	// 204 No Content: 성공했지만 돌려줄 본문은 없음 (삭제 시 표준)
	c.Status(http.StatusNoContent)
}
