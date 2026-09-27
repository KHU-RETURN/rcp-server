package functions

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ Svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{Svc: svc} }

func (h *Handler) InitRoutes(r *gin.RouterGroup) {
	d := r.Group("/databases")
	d.GET("", h.listDatabases)
	d.POST("", h.createDatabase)
	d.DELETE("/:db_id", h.deleteDatabase)
	d.POST("/:db_id/query", h.queryDatabase)
	f := r.Group("/functions")
	f.GET("", h.list)
	f.POST("", h.create)
	f.PUT("/:id", h.update)
	f.DELETE("/:id", h.delete)
	f.POST("/:id/invoke", h.invoke)
	f.POST("/:id/key", h.issueKey)
	f.DELETE("/:id/key", h.revokeKey)
	f.GET("/:id/databases", h.listBindings)
	f.PUT("/:id/databases/:alias", h.bindDatabase)
	f.DELETE("/:id/databases/:alias", h.unbindDatabase)
	f.GET("/:id/data/:collection", h.listData)
	f.GET("/:id/data/:collection/:key", h.getData)
	f.PUT("/:id/data/:collection/:key", h.putData)
	f.DELETE("/:id/data/:collection/:key", h.deleteData)
}

func ownerAndID(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	owner, ok := api.MustOwnerID(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid function id"})
		return uuid.Nil, uuid.Nil, false
	}
	return owner, id, true
}

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrDataNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidWasm), errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidExpiry), errors.Is(err, ErrInvalidData), errors.Is(err, ErrInvalidDatabase):
		status = http.StatusBadRequest
	case errors.Is(err, ErrInvalidKey):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrInvalidLanguage), errors.Is(err, ErrBuildFailed):
		status = http.StatusBadRequest
	case errors.Is(err, ErrBuildUnavailable), errors.Is(err, ErrDataUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, ErrConflict), errors.Is(err, ErrLimit), errors.Is(err, ErrDataLimit), errors.Is(err, ErrDatabaseLimit):
		status = http.StatusConflict
	case errors.Is(err, ErrBusy):
		status = http.StatusTooManyRequests
	case errors.Is(err, ErrTimeout):
		status = http.StatusGatewayTimeout
	case errors.Is(err, ErrOutputLimit), errors.Is(err, ErrInvalidOutput), errors.Is(err, ErrSQLLimit):
		status = http.StatusUnprocessableEntity
	}
	if status == http.StatusInternalServerError {
		c.JSON(status, api.ErrorResponse{Error: "function operation failed"})
		return
	}
	c.JSON(status, api.ErrorResponse{Error: err.Error()})
}

func (h *Handler) issueKey(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	var req struct {
		ExpiresInDays int `json:"expires_in_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid key expiry request"})
		return
	}
	issued, err := h.Svc.IssueKey(c.Request.Context(), owner, id, req.ExpiresInDays)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, issued)
}

func (h *Handler) revokeKey(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	if err := h.Svc.RevokeKey(c.Request.Context(), owner, id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func readUpload(c *gin.Context) ([]byte, string, bool, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxWasmBytes+(128<<10))
	language := c.PostForm("language")
	if language == "" {
		language = "wasm"
	}
	limit := MaxSourceBytes
	if language == "wasm" {
		limit = MaxWasmBytes
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "function file is required"})
		return nil, "", false, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) == 0 || len(data) > limit {
		c.JSON(http.StatusRequestEntityTooLarge, api.ErrorResponse{Error: "function file exceeds size limit"})
		return nil, "", false, false
	}
	return data, language, c.PostForm("data_mode") == "true", true
}

func (h *Handler) list(c *gin.Context) {
	owner, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	items, err := h.Svc.List(c.Request.Context(), owner)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) create(c *gin.Context) {
	owner, ok := api.MustOwnerID(c)
	if !ok {
		return
	}
	data, language, dataMode, ok := readUpload(c)
	if !ok {
		return
	}
	var fn *Function
	var err error
	if language == "wasm" {
		fn, err = h.Svc.Create(c.Request.Context(), owner, c.PostForm("name"), data, dataMode)
	} else {
		fn, err = h.Svc.CreateSource(c.Request.Context(), owner, c.PostForm("name"), language, data, dataMode)
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, fn)
}

func (h *Handler) update(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	data, language, dataMode, ok := readUpload(c)
	if !ok {
		return
	}
	var fn *Function
	var err error
	if language == "wasm" {
		fn, err = h.Svc.Update(c.Request.Context(), owner, id, data, dataMode)
	} else {
		fn, err = h.Svc.UpdateSource(c.Request.Context(), owner, id, language, data, dataMode)
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, fn)
}

func (h *Handler) delete(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	if err := h.Svc.Delete(c.Request.Context(), owner, id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) invoke(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxHTTPBodyBytes)
	input, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, api.ErrorResponse{Error: "input exceeds 65536 bytes"})
		return
	}
	result, err := h.Svc.Invoke(c.Request.Context(), owner, id, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) listData(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid data offset"})
			return
		}
	}
	items, err := h.Svc.ListData(c.Request.Context(), owner, id, c.Param("collection"), offset)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) getData(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	item, err := h.Svc.GetData(c.Request.Context(), owner, id, c.Param("collection"), c.Param("key"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) putData(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxDataValueBytes)
	value, err := io.ReadAll(c.Request.Body)
	if err != nil || !json.Valid(value) {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: ErrInvalidData.Error()})
		return
	}
	item, err := h.Svc.PutData(c.Request.Context(), owner, id, c.Param("collection"), c.Param("key"), value)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) deleteData(c *gin.Context) {
	owner, id, ok := ownerAndID(c)
	if !ok {
		return
	}
	if err := h.Svc.DeleteData(c.Request.Context(), owner, id, c.Param("collection"), c.Param("key")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
