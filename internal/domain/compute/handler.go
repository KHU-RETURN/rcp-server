package compute

import (
	"errors"
	"net/http"

	"github.com/KHU-RETURN/rcp-server/internal/api"
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
		// 조회 추가!
		computeGroup.GET("/instances", h.GetInstances)
		computeGroup.GET("/instances/:id", h.GetInstanceDetail)
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
	flavors, err := h.Svc.GetAvailableFlavorsWithLimit()
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

	server, err := h.Svc.CreateInstance(opts)
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
	serverID := c.Param("id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server ID is required"})
		return
	}

	if err := h.Svc.DeleteInstance(serverID); err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "해당 인스턴스를 찾을 수 없습니다.",
				"code":  "INSTANCE_NOT_FOUND",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "인스턴스 삭제 중 서버 오류가 발생했습니다.",
				"detail": err.Error(),
			})
		}
		return
	}

	c.Status(http.StatusNoContent)
}
