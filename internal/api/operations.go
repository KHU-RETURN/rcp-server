package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var ErrOperationConflict = errors.New("operation conflicts with an existing request")
var ErrOperationNotFound = errors.New("resource not found")
var ErrOperationInvalid = errors.New("invalid operation")

type OperationAccepted struct {
	OperationID uuid.UUID `json:"operation_id"`
	Status      string    `json:"status"`
}

type OperationQueue interface {
	Submit(context.Context, uuid.UUID, string, string, json.RawMessage, string) (*OperationAccepted, error)
}

// QueueOperation keeps request authentication and validation in the domain handler.
func QueueOperation(c *gin.Context, queue OperationQueue, owner uuid.UUID, kind, resource string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		c.JSON(500, ErrorResponse{Error: "invalid operation payload"})
		return
	}
	result, err := queue.Submit(c.Request.Context(), owner, kind, resource, data, c.GetHeader("Idempotency-Key"))
	if err != nil {
		code := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrOperationConflict):
			code = 409
		case errors.Is(err, ErrOperationNotFound):
			code = 404
		case errors.Is(err, ErrOperationInvalid):
			code = 400
		}
		c.JSON(code, ErrorResponse{Error: http.StatusText(code)})
		return
	}
	c.Header("Location", BasePath+"/operations/"+result.OperationID.String())
	c.JSON(http.StatusAccepted, result)
}
