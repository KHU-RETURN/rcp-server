package blockstorage

import (
	"errors"
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
	group := rg.Group("/block-storage")
	{
		group.GET("/volumes", h.ListVolumes)
		group.POST("/volumes", h.CreateVolume)
		group.GET("/volumes/:id", h.GetVolume)
		group.PATCH("/volumes/:id", h.UpdateVolume)
		group.DELETE("/volumes/:id", h.DeleteVolume)
		group.POST("/volumes/:id/attachments", h.AttachVolume)
		group.DELETE("/volumes/:id/attachments/:instanceId", h.DetachVolume)
		group.GET("/snapshots", h.ListSnapshots)
		group.POST("/volumes/:id/snapshots", h.CreateSnapshot)
		group.DELETE("/snapshots/:id", h.DeleteSnapshot)
	}
}

func (h *Handler) ListVolumes(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	volumes, err := h.Svc.ListVolumes(c.Request.Context(), ownerID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, VolumeListResponse{Volumes: volumes})
}

func (h *Handler) GetVolume(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	volume, err := h.Svc.GetVolume(c.Request.Context(), ownerID, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, volume)
}

func (h *Handler) CreateVolume(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	var req CreateVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	volume, err := h.Svc.CreateVolume(c.Request.Context(), ownerID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, volume)
}

func (h *Handler) UpdateVolume(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	var req UpdateVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	volume, err := h.Svc.UpdateVolume(c.Request.Context(), ownerID, c.Param("id"), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, volume)
}

func (h *Handler) DeleteVolume(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	if err := h.Svc.DeleteVolume(ownerID, c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) AttachVolume(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	var req AttachVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	response, err := h.Svc.AttachVolume(c.Request.Context(), ownerID, c.Param("id"), req.InstanceID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, response)
}

func (h *Handler) DetachVolume(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	response, err := h.Svc.DetachVolume(c.Request.Context(), ownerID, c.Param("id"), c.Param("instanceId"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, response)
}

func (h *Handler) ListSnapshots(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	snapshots, err := h.Svc.ListSnapshots(ownerID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, SnapshotListResponse{Snapshots: snapshots})
}

func (h *Handler) CreateSnapshot(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	var req CreateSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	snapshot, err := h.Svc.CreateSnapshot(ownerID, c.Param("id"), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, snapshot)
}

func (h *Handler) DeleteSnapshot(c *gin.Context) {
	ownerID, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	if err := h.Svc.DeleteSnapshot(ownerID, c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrVolumeNotFound), errors.Is(err, ErrSnapshotNotFound), errors.Is(err, ErrInstanceNotFound), errors.Is(err, ErrVolumeAttachmentNotFound):
		c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
	case errors.Is(err, ErrVolumeNameRequired), errors.Is(err, ErrVolumeSizeInvalid), errors.Is(err, ErrSnapshotNameRequired), errors.Is(err, ErrSnapshotSmallerThanSource):
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
	case errors.Is(err, ErrVolumeNotAvailable), errors.Is(err, ErrVolumeAttached), errors.Is(err, ErrSnapshotBusy):
		c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
	}
}
