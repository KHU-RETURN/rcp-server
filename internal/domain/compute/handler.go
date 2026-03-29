package compute

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
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
	flavors, err := h.Svc.GetFlavors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "사양 조회를 실패했습니다: " + err.Error()})
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

func (h *Handler) GetAvailableFlavors(c *gin.Context) {
	// 인프라 레이어를 직접 안 부르고 Service(또는 Repo)를 거칩니다.
	client, err := h.Svc.GetComputeClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cloud connection failed"})
		return
	}

	projectID := os.Getenv("OS_PROJECT_ID")

	flavors, err := h.Svc.GetAvailableFlavorsWithLimit(client, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, flavors)
}

func (h *Handler) CreateServer(c *gin.Context) {
	// 1. 요청 바디를 담을 구조체 정의
	var req struct {
		Name      string `json:"name" binding:"required"`
		ImageRef  string `json:"image_id" binding:"required"`
		FlavorRef string `json:"flavor_id" binding:"required"`
		// 네트워크 ID가 필요한 경우를 대비해 추가 (선택사항)
		NetworkID string `json:"network_id"`
	}

	// 2. JSON 바인딩 및 유효성 검사
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// 3. 서비스 호출을 위한 클라이언트 준비
	client, err := h.Svc.GetComputeClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to cloud"})
		return
	}

	// 4. 서비스 레이어 옵션 조립
	opts := CreateServerOpts{
		Name:      req.Name,
		ImageRef:  req.ImageRef,
		FlavorRef: req.FlavorRef,
	}

	// 네트워크 ID가 입력되었다면 리스트에 추가
	if req.NetworkID != "" {
		opts.Networks = []servers.Network{{UUID: req.NetworkID}}
	}

	// 5. 서버 생성 실행
	server, err := h.Svc.CreateInstance(client, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 6. 생성 요청 성공 (201 Created)
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
		// 아까 배운 대로! 없는 서버면 404, 아니면 500
		if strings.Contains(err.Error(), "찾을 수 없습니다") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 204 No Content: 성공했지만 돌려줄 본문은 없음 (삭제 시 표준)
	c.Status(http.StatusNoContent)
}
