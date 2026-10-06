package operations

import (
	"encoding/json"
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/outboxevent"
	"github.com/KHU-RETURN/rcp-server/ent/resourceoperation"
	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"strconv"
	"time"
)

type Handler struct{ Svc *Service }

func (h *Handler) InitRoutes(g *gin.RouterGroup) {
	g.GET("/operations/:id", h.Get)
	g.GET("/operations/:id/events", h.Events)
}
func (h *Handler) InitAdminRoutes(g *gin.RouterGroup) {
	g.GET("/admin/reconciliation", h.Observations)
	g.GET("/admin/operations", h.List)
	g.GET("/admin/notifications", h.Notifications)
}

type operationResponse struct {
	ID         uuid.UUID `json:"id"`
	Kind       string    `json:"kind"`
	ResourceID string    `json:"resource_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastError  string    `json:"last_error,omitempty"`
}

func view(op *ent.ResourceOperation) operationResponse {
	reason := ""
	if op.LastError != "" {
		reason = "operation requires retry or review"
	}
	return operationResponse{op.ID, op.Kind, op.ResourceID, op.Status, op.CreatedAt, op.UpdatedAt, reason}
}
func (h *Handler) owned(c *gin.Context) *ent.ResourceOperation {
	owner, ok := api.OwnerID(c)
	if !ok {
		c.AbortWithStatus(401)
		return nil
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(400)
		return nil
	}
	op, err := h.Svc.db.ResourceOperation.Query().Where(resourceoperation.ID(id), resourceoperation.OwnerID(owner)).Only(c.Request.Context())
	if ent.IsNotFound(err) {
		c.AbortWithStatus(404)
		return nil
	}
	if err != nil {
		c.AbortWithStatus(500)
		return nil
	}
	return op
}
func (h *Handler) Get(c *gin.Context) {
	if op := h.owned(c); op != nil {
		c.JSON(200, view(op))
	}
}
func (h *Handler) Events(c *gin.Context) {
	op := h.owned(c)
	if op == nil {
		return
	}
	rows, err := h.Svc.db.OutboxEvent.Query().Where(outboxevent.OperationID(op.ID), outboxevent.KindNEQ("execute")).Order(ent.Desc(outboxevent.FieldCreatedAt)).Limit(100).All(c.Request.Context())
	if err != nil {
		c.Status(500)
		return
	}
	c.JSON(200, eventViews(rows))
}
func (h *Handler) List(c *gin.Context) {
	offset, limit := page(c)
	rows, err := h.Svc.db.ResourceOperation.Query().Order(ent.Desc(resourceoperation.FieldCreatedAt)).Offset(offset).Limit(limit).All(c.Request.Context())
	if err != nil {
		c.Status(500)
		return
	}
	result := make([]operationResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, view(row))
	}
	c.JSON(200, result)
}
func (h *Handler) Observations(c *gin.Context) {
	offset, limit := page(c)
	rows, err := h.Svc.db.ResourceObservation.Query().Order(ent.Asc("resource_key")).Offset(offset).Limit(limit).All(c.Request.Context())
	if err != nil {
		c.Status(500)
		return
	}
	c.JSON(http.StatusOK, rows)
}
func (h *Handler) Notifications(c *gin.Context) {
	offset, limit := page(c)
	rows, err := h.Svc.db.OutboxEvent.Query().Where(outboxevent.KindNEQ("execute"), outboxevent.ProcessedAtNotNil()).Order(ent.Desc(outboxevent.FieldCreatedAt)).Offset(offset).Limit(limit).All(c.Request.Context())
	if err != nil {
		c.Status(500)
		return
	}
	c.JSON(200, eventViews(rows))
}

type eventResponse struct {
	ID          uuid.UUID       `json:"id"`
	OperationID uuid.UUID       `json:"operation_id"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Attempts    int             `json:"attempts"`
	ProcessedAt *time.Time      `json:"processed_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

func eventViews(rows []*ent.OutboxEvent) []eventResponse {
	result := make([]eventResponse, 0, len(rows))
	for _, r := range rows {
		result = append(result, eventResponse{r.ID, r.OperationID, r.Kind, json.RawMessage(r.Payload), r.Attempts, r.ProcessedAt, r.CreatedAt})
	}
	return result
}
func page(c *gin.Context) (int, int) {
	p, _ := strconv.Atoi(c.Query("page"))
	if p < 1 || p > 1000000 {
		p = 1
	}
	n, _ := strconv.Atoi(c.Query("limit"))
	if n < 1 {
		n = 100
	}
	if n > 200 {
		n = 200
	}
	return (p - 1) * n, n
}
