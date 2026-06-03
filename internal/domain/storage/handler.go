package storage

import (
	"errors"
	"fmt"
	"io"
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
		g.GET("/containers/:name/archive", h.ArchiveObjects)
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

	reader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing file"})
		return
	}

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid multipart body"})
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}

		fileStream, contentType := resolveObjectContentStream(
			part,
			part.Header.Get(api.HeaderContentType),
			part.FileName(),
			objectName,
		)

		if err := h.Svc.UploadObject(c.Request.Context(), id, containerName, objectName, fileStream, contentType); err != nil {
			_ = part.Close()
			switch {
			case errors.Is(err, ErrContainerNotFound):
				c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
			default:
				c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
			}
			return
		}
		_ = part.Close()
		c.JSON(http.StatusCreated, UploadObjectResponse{Key: objectName})
		return
	}

	c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing file"})
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

func (h *Handler) ArchiveObjects(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	containerName := c.Param("name")
	prefix := normalizeObjectPrefix(c.Query("prefix"))

	c.Header(api.HeaderContentType, "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", archiveFilename(containerName, prefix)))
	if err := h.Svc.ArchiveObjects(c.Request.Context(), id, containerName, prefix, c.Writer); err != nil {
		if c.Writer.Written() {
			_ = c.Error(err)
			return
		}
		switch {
		case errors.Is(err, ErrContainerNotFound), errors.Is(err, ErrObjectPrefixNotFound):
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

func archiveFilename(containerName, prefix string) string {
	base := strings.TrimSuffix(prefix, "/")
	if base == "" {
		base = containerName
	} else if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	if base == "" {
		base = "objects"
	}
	return base + ".zip"
}
