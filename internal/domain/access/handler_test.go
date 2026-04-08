package access

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestHandlerCreateKeyPair(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(repo *fakeClient) *Handler {
		return NewHandler(newTestService(repo))
	}
	newRouter := func(handler *Handler) *gin.Engine {
		r := gin.New()
		v1 := r.Group(api.BasePath)
		v1.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyUser, &auth.User{ID: accessTestUserID})
			c.Next()
		})
		handler.InitRoutes(v1)
		return r
	}

	t.Run("returns 201 with response body", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return &KeyPair{Name: name, Fingerprint: "fp", PublicKey: publicKey}, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}

		var res KeyPairResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if res.Name != "key" || res.Fingerprint != "fp" || res.PublicKey != testPublicKey {
			t.Fatalf("unexpected response body: %+v", res)
		}
	})

	t.Run("returns 400 for invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := newRouter(newHandler(&fakeClient{}))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}

		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "Invalid request body" {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})

	t.Run("returns 409 for duplicate name", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return &KeyPair{Name: name}, nil
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected status 409, got %d", w.Code)
		}

		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "name already exists" {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})

	t.Run("returns 403 for upstream access denied", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusForbidden)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				t.Fatal("create should not be called when lookup is forbidden")
				return nil, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", w.Code)
		}

		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "keypair access denied" {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})

	t.Run("returns 400 for invalid keypair request", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusBadRequest)
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "invalid keypair request: Bad Request" {
			t.Fatalf("unexpected error message: %q", res.Error)
		}
	})

	t.Run("returns 500 default for unclassified error", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				// Return a raw error that doesn't match any classified error
				return nil, errors.New("unclassified internal error")
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		// Non-StatusError wraps as ErrKeyPairOperationFailed → 500
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})

	t.Run("returns 500 with sanitized message for internal failures", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, errors.New("provider bootstrap leaked")
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "provider bootstrap leaked") {
			t.Fatalf("response leaked provider details: %s", w.Body.String())
		}

		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != internalServerErrorMessage {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})
}

func TestHandlerGetAndDeleteKeyPair(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newRouter := func(handler *Handler) *gin.Engine {
		r := gin.New()
		v1 := r.Group(api.BasePath)
		v1.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyUser, &auth.User{ID: accessTestUserID})
			c.Next()
		})
		handler.InitRoutes(v1)
		return r
	}

	t.Run("returns 404 for upstream not found on get", func(t *testing.T) {
		client := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
		}
		repo := &noopKeyPairRepository{
			isOwnerFn: func(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
				return true, nil
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/access/keypairs/team-default-key", nil)
		w := httptest.NewRecorder()
		r := newRouter(NewHandler(newTestServiceWithRepo(client, repo)))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", w.Code)
		}
	})

	t.Run("returns 500 for ownership lookup failures on delete", func(t *testing.T) {
		repo := &noopKeyPairRepository{
			isOwnerFn: func(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
				return false, errors.New("sqlite busy leaked")
			},
		}

		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/access/keypairs/team-default-key", nil)
		w := httptest.NewRecorder()
		r := newRouter(NewHandler(newTestServiceWithRepo(&fakeClient{}, repo)))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "sqlite busy leaked") {
			t.Fatalf("response leaked internal error: %s", w.Body.String())
		}
	})
}
