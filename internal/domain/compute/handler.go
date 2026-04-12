package compute

import (
	"errors"
	"net/http"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	Svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

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

func (h *Handler) GetInstances(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	instances, err := h.Svc.GetInstances(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, instances)
}

func (h *Handler) GetInstanceDetail(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	instanceID := c.Param("id")
	detail, err := h.Svc.GetInstanceDetail(c.Request.Context(), id, instanceID)
	if err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *Handler) CreateInstance(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
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
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		}
		return
	}

	server, err := h.Svc.CreateInstance(c.Request.Context(), id, opts)
	if err != nil {
		switch {
		case errors.Is(err, ErrCreateInstanceNameRequired),
			errors.Is(err, ErrCreateInstanceImageRequired),
			errors.Is(err, ErrCreateInstanceFlavorRequired):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, server)
}

func (h *Handler) DeleteInstance(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	serverID := c.Param("id")
	if err := h.Svc.DeleteInstance(c.Request.Context(), id, serverID); err != nil {
		if errors.Is(err, ErrInstanceNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
