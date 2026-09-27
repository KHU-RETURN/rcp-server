package functions

import (
	"net/http"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func databaseID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("db_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid database id"})
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) listDatabases(c *gin.Context) {
	owner, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	items, err := h.Svc.ListDatabases(c.Request.Context(), owner)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) createDatabase(c *gin.Context) {
	owner, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid database request"})
		return
	}
	item, err := h.Svc.CreateDatabase(c.Request.Context(), owner, req.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *Handler) deleteDatabase(c *gin.Context) {
	owner, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	id, ok := databaseID(c)
	if !ok {
		return
	}
	if err := h.Svc.DeleteDatabase(c.Request.Context(), owner, id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) queryDatabase(c *gin.Context) {
	owner, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	id, ok := databaseID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid SQL request"})
		return
	}
	result, err := h.Svc.QueryDatabase(c.Request.Context(), owner, id, req.SQL, req.Params)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) listBindings(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	items, err := h.Svc.ListBindings(c.Request.Context(), owner, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) bindDatabase(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	var req struct {
		DatabaseID uuid.UUID `json:"database_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.DatabaseID == uuid.Nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid database binding"})
		return
	}
	if err := h.Svc.BindDatabase(c.Request.Context(), owner, id, req.DatabaseID, c.Param("alias")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) unbindDatabase(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	if err := h.Svc.UnbindDatabase(c.Request.Context(), owner, id, c.Param("alias")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
