package apps

import (
	"errors"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/KHU-RETURN/rcp-server/internal/api"
)

const (
	envAppBaseDomain = "RCP_APP_BASE_DOMAIN"

	defaultAppBaseDomainValue = "apps.khu-return.com"
)

type Handler struct {
	Svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	computeGroup := rg.Group("/compute")
	{
		computeGroup.POST("/instances/:id/app", h.RegisterApp)
		computeGroup.DELETE("/instances/:id/app", h.DeleteApp)
	}
}

func (h *Handler) RegisterApp(c *gin.Context) {
	ownerID, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req RegisterAppRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		return
	}

	res, err := h.Svc.RegisterApp(c.Request.Context(), ownerID, c.Param("id"), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrBaseDomainRequired),
			errors.Is(err, ErrSubdomainRequired),
			errors.Is(err, ErrInvalidSubdomain):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrInstanceNotFound):
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrAppAlreadyExists):
			c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to register app"})
		}
		return
	}

	c.JSON(http.StatusCreated, res)
}

func (h *Handler) DeleteApp(c *gin.Context) {
	ownerID, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	if err := h.Svc.DeleteApp(c.Request.Context(), ownerID, c.Param("id")); err != nil {
		if errors.Is(err, ErrAppNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to delete app"})
		return
	}

	c.Status(http.StatusNoContent)
}

func defaultAppBaseDomain() string {
	if domain := os.Getenv(envAppBaseDomain); domain != "" {
		return domain
	}
	return defaultAppBaseDomainValue
}
