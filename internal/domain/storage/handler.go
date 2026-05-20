package storage

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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
	g := rg.Group("/storage")
	{
		g.GET("/containers", h.ListContainers)
		g.POST("/containers", h.CreateContainer)
		g.DELETE("/containers/:name", h.DeleteContainer)
		g.GET("/containers/:name/objects", h.ListObjects)
		g.POST("/containers/:name/objects/*key", h.UploadObject)
		g.GET("/containers/:name/objects/*key", h.DownloadObject)
		g.DELETE("/containers/:name/objects/*key", h.DeleteObject)
	}
}

func (h *Handler) ListContainers(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	containers, err := h.Svc.ListContainers(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, containers)
}

func (h *Handler) CreateContainer(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req CreateContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}

	res, err := h.Svc.CreateContainer(c.Request.Context(), id, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrContainerAlreadyExists):
			c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, res)
}

func (h *Handler) DeleteContainer(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	name := c.Param("name")
	force, _ := strconv.ParseBool(c.Query("force"))

	if err := h.Svc.DeleteContainer(c.Request.Context(), id, name, force); err != nil {
		switch {
		case errors.Is(err, ErrContainerNotFound):
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrContainerNotEmpty):
			c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ListObjects(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	containerName := c.Param("name")
	objs, err := h.Svc.ListObjects(c.Request.Context(), id, containerName)
	if err != nil {
		switch {
		case errors.Is(err, ErrContainerNotFound):
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, objs)
}

func (h *Handler) UploadObject(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	containerName := c.Param("name")
	objectName := strings.TrimPrefix(c.Param("key"), "/")
	if objectName == "" {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing object key"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing file"})
		return
	}
	defer func() { _ = file.Close() }()

	contentType := header.Header.Get(api.HeaderContentType)

	if err := h.Svc.UploadObject(c.Request.Context(), id, containerName, objectName, file, contentType); err != nil {
		switch {
		case errors.Is(err, ErrContainerNotFound):
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, UploadObjectResponse{Key: objectName})
}

func (h *Handler) DownloadObject(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	containerName := c.Param("name")
	objectName := strings.TrimPrefix(c.Param("key"), "/")

	if err := h.Svc.DownloadObject(c.Request.Context(), id, containerName, objectName, c.Writer); err != nil {
		switch {
		case errors.Is(err, ErrContainerNotFound):
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		}
		return
	}
}

func (h *Handler) DeleteObject(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	containerName := c.Param("name")
	objectName := strings.TrimPrefix(c.Param("key"), "/")

	if err := h.Svc.DeleteObject(c.Request.Context(), id, containerName, objectName); err != nil {
		switch {
		case errors.Is(err, ErrContainerNotFound):
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
