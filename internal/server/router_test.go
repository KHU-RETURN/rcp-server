package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/KHU-RETURN/rcp-server/internal/domain/storage"
)

func TestNewRouterRegistersComputeRoutes(t *testing.T) {
	setGinMode(t, gin.TestMode)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Auth:    &auth.Handler{},
		Compute: &compute.Handler{},
		Storage: &storage.Handler{},
	})

	routes := router.Routes()
	var foundFlavors bool
	var foundCreateInstance bool
	var foundAuthLogin bool
	var foundAuthMe bool
	var foundConsoleWebSocket bool
	var foundAuthorizedKeys bool
	var foundStorageContainers bool
	var foundStorageUploadObject bool
	var foundStorageArchiveObjects bool

	for _, route := range routes {
		if route.Method == http.MethodGet && route.Path == api.BasePath+"/auth/me" {
			foundAuthMe = true
		}
		if route.Method == http.MethodGet && route.Path == api.BasePath+"/auth/oauth/google" {
			foundAuthLogin = true
		}
		if route.Method == http.MethodGet && route.Path == api.BasePath+"/compute/flavors" {
			foundFlavors = true
		}
		if route.Method == http.MethodPost && route.Path == api.BasePath+"/compute/instances" {
			foundCreateInstance = true
		}
		if route.Method == http.MethodGet && route.Path == api.BasePath+"/access/console/ws" {
			foundConsoleWebSocket = true
		}
		if route.Method == http.MethodGet && route.Path == api.BasePath+"/internal/ssh/authorized-keys" {
			foundAuthorizedKeys = true
		}
		if route.Method == http.MethodGet && route.Path == api.BasePath+"/storage/containers" {
			foundStorageContainers = true
		}
		if route.Method == http.MethodPost && route.Path == api.BasePath+"/storage/containers/:name/objects/*key" {
			foundStorageUploadObject = true
		}
		if route.Method == http.MethodGet && route.Path == api.BasePath+"/storage/containers/:name/archive" {
			foundStorageArchiveObjects = true
		}
	}

	if !foundFlavors {
		t.Fatalf("%s %s/compute/flavors route was not registered", http.MethodGet, api.BasePath)
	}
	if !foundCreateInstance {
		t.Fatalf("%s %s/compute/instances route was not registered", http.MethodPost, api.BasePath)
	}
	if !foundAuthLogin {
		t.Fatalf("%s %s/auth/oauth/google route was not registered", http.MethodGet, api.BasePath)
	}
	if !foundAuthMe {
		t.Fatalf("%s %s/auth/me route was not registered", http.MethodGet, api.BasePath)
	}
	if !foundConsoleWebSocket {
		t.Fatalf("%s %s/access/console/ws route was not registered", http.MethodGet, api.BasePath)
	}
	if !foundAuthorizedKeys {
		t.Fatalf("%s %s/internal/ssh/authorized-keys route was not registered", http.MethodGet, api.BasePath)
	}
	if !foundStorageContainers {
		t.Fatalf("%s %s/storage/containers route was not registered", http.MethodGet, api.BasePath)
	}
	if !foundStorageUploadObject {
		t.Fatalf("%s %s/storage/containers/:name/objects route was not registered", http.MethodPost, api.BasePath)
	}
	if !foundStorageArchiveObjects {
		t.Fatalf("%s %s/storage/containers/:name/archive route was not registered", http.MethodGet, api.BasePath)
	}
}

func TestRouterCORSAllowsFrontendOrigin(t *testing.T) {
	setGinMode(t, gin.TestMode)
	t.Setenv(envAllowedOrigins, "https://frontend.example.com")

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Auth:    &auth.Handler{},
		Compute: &compute.Handler{},
		Storage: &storage.Handler{},
	})

	req := httptest.NewRequest(http.MethodOptions, api.BasePath+"/auth/me", nil)
	req.Header.Set("Origin", "https://frontend.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://frontend.example.com" {
		t.Fatalf("expected allowed origin, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials to be allowed, got %q", got)
	}
}

func TestRouterCORSAllowsPatternOrigin(t *testing.T) {
	setGinMode(t, gin.TestMode)
	t.Setenv(envAllowedOriginPattern, `^https://preview-[0-9]+[.]frontend[.]example[.]com$`)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Auth:    &auth.Handler{},
		Compute: &compute.Handler{},
		Storage: &storage.Handler{},
	})

	req := httptest.NewRequest(http.MethodOptions, api.BasePath+"/auth/me", nil)
	req.Header.Set("Origin", "https://preview-21.frontend.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://preview-21.frontend.example.com" {
		t.Fatalf("expected allowed origin, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials to be allowed, got %q", got)
	}
}

func TestRouterCORSRejectsPatternLookalikeOrigin(t *testing.T) {
	setGinMode(t, gin.TestMode)
	t.Setenv(envAllowedOriginPattern, `^https://preview-[0-9]+[.]frontend[.]example[.]com$`)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Auth:    &auth.Handler{},
		Compute: &compute.Handler{},
		Storage: &storage.Handler{},
	})

	req := httptest.NewRequest(http.MethodOptions, api.BasePath+"/auth/me", nil)
	req.Header.Set("Origin", "https://preview-21.frontend.example.com.evil.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allowed origin, got %q", got)
	}
}

func TestRouterCORSRejectsUnconfiguredOrigin(t *testing.T) {
	setGinMode(t, gin.TestMode)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Auth:    &auth.Handler{},
		Compute: &compute.Handler{},
		Storage: &storage.Handler{},
	})

	req := httptest.NewRequest(http.MethodOptions, api.BasePath+"/auth/me", nil)
	req.Header.Set("Origin", "https://frontend.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allowed origin, got %q", got)
	}
}

func setGinMode(t *testing.T, mode string) {
	t.Helper()

	previous := gin.Mode()
	gin.SetMode(mode)
	t.Cleanup(func() {
		gin.SetMode(previous)
	})
}
