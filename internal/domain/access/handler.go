package access

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
	accessGroup := rg.Group("/access")
	{
		accessGroup.POST("/keypairs", h.CreateKeyPair)
	}
}

// CreateKeyPair godoc
// @Summary Create an OpenStack key pair
// @Description Creates a key pair after validating the request body and SSH public key format.
// @Tags access
// @Accept json
// @Produce json
// @Param request body CreateKeyPairRequest true "Key pair request"
// @Success 201 {object} KeyPairResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Router /api/v1/access/keypairs [post]
func (h *Handler) CreateKeyPair(c *gin.Context) {
	var req CreateKeyPairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		return
	}

	res, err := h.Svc.CreateKeyPair(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNameRequired), errors.Is(err, ErrPublicKeyRequired), errors.Is(err, ErrInvalidSSHKeyFormat):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrInvalidKeyPairRequest):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid keypair request"})
		case errors.Is(err, ErrKeyPairAccessDenied):
			c.JSON(http.StatusForbidden, api.ErrorResponse{Error: "keypair access denied"})
		case errors.Is(err, ErrKeyPairAlreadyExists):
			c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrKeyPairOperationFailed):
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to create keypair"})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "internal server error"})
		}
		return
	}

	c.JSON(http.StatusCreated, res)
}
