package compute

import (
	"errors"
	"net/http"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func getUserID(c *gin.Context) (uuid.UUID, error) {
	value, ok := c.Get(auth.ContextKeyUser)
	if !ok {
		value, ok = c.Get("currentUser")
	}
	if !ok {
		return uuid.Nil, errors.New("unauthorized")
	}

	currentUser, ok := value.(*auth.User)
	if !ok || currentUser == nil {
		return uuid.Nil, errors.New("unauthorized")
	}

	return currentUser.ID, nil
}

func (h *Handler) GetFlavors(c *gin.Context) {
	if c.Query("available") == "true" {
		flavors, err := h.Svc.GetAvailableFlavorsWithLimit()
		if err != nil {
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, flavors)
		return
	}

	flavors, err := h.Svc.GetFlavors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "사양 조회를 실패했습니다: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, flavors)
}

// GetInstanceDetail 핸들러
func (h *Handler) GetInstanceDetail(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "인스턴스 ID가 필요합니다."})
		return
	}

	detail, err := h.Svc.GetInstanceDetail(c.Request.Context(), userID, id)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "해당 인스턴스를 찾을 수 없습니다."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "상세 조회 실패: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, detail)
}

func (h *Handler) GetInstances(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	instances, err := h.Svc.GetInstances(c.Request.Context(), userID)
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
		computeGroup.GET("/flavors", h.GetFlavors)
		computeGroup.GET("/instances", h.GetInstances)
		computeGroup.GET("/instances/:id", h.GetInstanceDetail)
		computeGroup.POST("/instances", h.CreateInstance)
		computeGroup.DELETE("/instances/:id", h.DeleteInstance)
	}
}

func (h *Handler) CreateInstance(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

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

	server, err := h.Svc.CreateInstance(c.Request.Context(), userID, opts)
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

func (h *Handler) DeleteInstance(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	serverID := c.Param("id")
	if serverID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server ID is required"})
		return
	}

	if err := h.Svc.DeleteInstance(c.Request.Context(), userID, serverID); err != nil {
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
