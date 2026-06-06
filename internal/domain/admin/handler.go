package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/KHU-RETURN/rcp-server/internal/api"
)

type Handler struct {
	Svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	adminGroup := rg.Group("/admin")
	{
		adminGroup.GET("/summary", h.Summary)
		adminGroup.GET("/users", h.Users)
		adminGroup.GET("/instances", h.Instances)
		adminGroup.GET("/system", h.System)
	}
}

func (h *Handler) Summary(c *gin.Context) {
	res, err := h.Svc.Summary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Users(c *gin.Context) {
	res, err := h.Svc.Users(c.Request.Context(), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Instances(c *gin.Context) {
	res, err := h.Svc.Instances(c.Request.Context(), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) System(c *gin.Context) {
	c.JSON(http.StatusOK, h.Svc.System())
}
