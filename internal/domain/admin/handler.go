package admin

import (
	"net/http"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
		adminGroup.GET("/users/:id/resources", h.UserResources)
		adminGroup.GET("/instances", h.Instances)
		adminGroup.GET("/instances/:id", h.Instance)
		adminGroup.GET("/containers", h.Containers)
		adminGroup.GET("/containers/:id", h.Container)
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
	res, err := h.Svc.Users(c.Request.Context(), c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Instances(c *gin.Context) {
	res, err := h.Svc.Instances(c.Request.Context(), c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Instance(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid instance id"})
		return
	}

	res, err := h.Svc.Instance(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: "instance not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Containers(c *gin.Context) {
	res, err := h.Svc.Containers(c.Request.Context(), c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) Container(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid container id"})
		return
	}

	res, err := h.Svc.Container(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: "container not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) UserResources(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid user id"})
		return
	}

	res, err := h.Svc.UserResources(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handler) System(c *gin.Context) {
	c.JSON(http.StatusOK, h.Svc.System(c.Request.Context()))
}
