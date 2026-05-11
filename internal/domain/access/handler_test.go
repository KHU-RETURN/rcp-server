package access

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/net/websocket"
)

var testUser = &auth.User{ID: testOwnerID}

func withTestUser(r *gin.RouterGroup) {
	r.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, testUser)
		c.Next()
	})
}

func TestWebsocketBaseURLUsesConfiguredURL(t *testing.T) {
	t.Setenv(envWebConsoleBaseURL, "https://console.example.test/")

	c := newTestGinContext("http://internal.local/console")

	got := websocketBaseURL(c)
	want := "wss://console.example.test" + api.BasePath
	if got != want {
		t.Fatalf("websocketBaseURL() = %q, want %q", got, want)
	}
}

func TestWebsocketBaseURLIgnoresForwardedHost(t *testing.T) {
	c := newTestGinContext("http://api.example.test/console")
	c.Request.Header.Set("X-Forwarded-Host", "attacker.example.test")
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	got := websocketBaseURL(c)
	want := "ws://api.example.test" + api.BasePath
	if got != want {
		t.Fatalf("websocketBaseURL() = %q, want %q", got, want)
	}
}

func TestValidateWebSocketOrigin(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins string
		origin         string
		host           string
		wantAllowed    bool
	}{
		{
			name:        "allows same host when allowed origins are not configured",
			origin:      "https://api.example.test",
			host:        "api.example.test",
			wantAllowed: true,
		},
		{
			name:        "rejects different host when allowed origins are not configured",
			origin:      "https://attacker.example.test",
			host:        "api.example.test",
			wantAllowed: false,
		},
		{
			name:           "allows configured origin",
			allowedOrigins: "https://console.example.test",
			origin:         "https://console.example.test",
			host:           "api.example.test",
			wantAllowed:    true,
		},
		{
			name:           "rejects unconfigured origin",
			allowedOrigins: "https://console.example.test",
			origin:         "https://attacker.example.test",
			host:           "api.example.test",
			wantAllowed:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envWebConsoleAllowedOrigins, tt.allowedOrigins)

			origin, err := url.Parse(tt.origin)
			if err != nil {
				t.Fatalf("parse origin: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/console", nil)
			cfg := &websocket.Config{Origin: origin}

			err = validateWebSocketOrigin(cfg, req)
			if tt.wantAllowed && err != nil {
				t.Fatalf("expected origin to be allowed, got %v", err)
			}
			if !tt.wantAllowed && err == nil {
				t.Fatal("expected origin to be rejected")
			}
		})
	}
}

func newTestGinContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
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
