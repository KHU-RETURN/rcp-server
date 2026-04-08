package access

import (
	"errors"
	"net/http"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	Svc *Service
}

const internalServerErrorMessage = "internal server error"

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func getUserID(c *gin.Context) (uuid.UUID, error) {
	value, ok := c.Get(auth.ContextKeyUser)
	if !ok {
		value, ok = c.Get("currentUser")
	}
	if !ok {
		return uuid.Nil, errors.New("unauthorized")
	}

	currentUser, ok := value.(*auth.User)
	if !ok || currentUser == nil {
		return uuid.Nil, errors.New("unauthorized")
	}

	return currentUser.ID, nil
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	accessGroup := rg.Group("/access")
	{
		accessGroup.GET("/keypairs", h.ListKeyPairs)
		accessGroup.POST("/keypairs", h.CreateKeyPair)
		accessGroup.GET("/keypairs/:name", h.GetKeyPair)
		accessGroup.DELETE("/keypairs/:name", h.DeleteKeyPair)
		// PUT/PATCH 미제공: SSH 키페어는 수정이 불가능하며, 변경 시 삭제 후 재생성이 표준입니다.
	}
}

func (h *Handler) ListKeyPairs(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	kps, err := h.Svc.ListKeyPairs(c.Request.Context(), userID)
	if err != nil {
		respondInternalServerError(c)
		return
	}
	c.JSON(http.StatusOK, kps)
}

func (h *Handler) GetKeyPair(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	name := c.Param("name")
	kp, err := h.Svc.GetKeyPair(c.Request.Context(), userID, name)
	if err != nil {
		if errors.Is(err, ErrKeyPairNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: ErrKeyPairNotFound.Error()})
			return
		}
		respondInternalServerError(c)
		return
	}
	c.JSON(http.StatusOK, kp)
}

func (h *Handler) DeleteKeyPair(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	name := c.Param("name")
	if err := h.Svc.DeleteKeyPair(c.Request.Context(), userID, name); err != nil {
		if errors.Is(err, ErrKeyPairNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: ErrKeyPairNotFound.Error()})
			return
		}
		respondInternalServerError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) CreateKeyPair(c *gin.Context) {
	userID, err := getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req CreateKeyPairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		return
	}

	res, err := h.Svc.CreateKeyPair(c.Request.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNameRequired), errors.Is(err, ErrPublicKeyRequired), errors.Is(err, ErrInvalidSSHKeyFormat):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrInvalidKeyPairRequest):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrKeyPairAccessDenied):
			c.JSON(http.StatusForbidden, api.ErrorResponse{Error: "keypair access denied"})
		case errors.Is(err, ErrKeyPairAlreadyExists):
			c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
		default:
			respondInternalServerError(c)
		}
		return
	}

	c.JSON(http.StatusCreated, res)
}

func respondInternalServerError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: internalServerErrorMessage})
}
