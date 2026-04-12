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

var testUser = &auth.User{ID: testOwnerID}

func withTestUser(r *gin.RouterGroup) {
	r.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, testUser)
		c.Next()
	})
}

func TestHandlerCreateKeyPair(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(osClient *fakeClient, repo *fakeRepo) *Handler {
		return NewHandler(NewService(osClient, repo))
	}

	t.Run("returns 201 with response body", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return &KeyPair{Name: name, Fingerprint: "fp", PublicKey: publicKey}, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(osClient, &fakeRepo{}).InitRoutes(v1)
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

	t.Run("returns 401 when user is not authenticated", func(t *testing.T) {
		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(&fakeClient{}, &fakeRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("returns 400 for invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(&fakeClient{}, &fakeRepo{}).InitRoutes(v1)
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
		repo := &fakeRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, name string) (*KeyPair, error) {
				return &KeyPair{Name: name}, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(&fakeClient{}, repo).InitRoutes(v1)
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

	t.Run("returns 403 with sanitized message for upstream access denied", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusForbidden)
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(osClient, &fakeRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "provider-secret") {
			t.Fatalf("response leaked provider details: %s", w.Body.String())
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
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusBadRequest)
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(osClient, &fakeRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "invalid keypair request" {
			t.Fatalf("unexpected error message: %q", res.Error)
		}
	})

	t.Run("returns 500 default for unclassified error", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, errors.New("unclassified internal error")
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(osClient, &fakeRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})

	t.Run("returns 500 with sanitized message for internal failures", func(t *testing.T) {
		repo := &fakeRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*KeyPair, error) {
				return nil, errors.New("provider bootstrap leaked")
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(&fakeClient{}, repo).InitRoutes(v1)
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
		if res.Error != "failed to create keypair" {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})
}
