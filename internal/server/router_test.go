package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/gin-gonic/gin"
)

func TestNewRouterRegistersComputeRoutes(t *testing.T) {
	setGinMode(t, gin.TestMode)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Compute: &compute.Handler{},
	})

	routes := router.Routes()
	for _, route := range routes {
		if route.Method == "GET" && route.Path == "/api/v1/compute/flavors" {
			return
		}
	}

	t.Fatalf("GET /api/v1/compute/flavors route was not registered")
}

func TestNewRouterServesDocsOutsideReleaseMode(t *testing.T) {
	setGinMode(t, gin.TestMode)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Compute: &compute.Handler{},
	})

	t.Run("serves scalar docs html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/docs", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Fatalf("expected html content type, got %q", got)
		}
		body := w.Body.String()
		if !strings.Contains(body, "https://cdn.jsdelivr.net/npm/@scalar/api-reference") {
			t.Fatalf("docs html missing scalar script: %s", body)
		}
		if !strings.Contains(body, "url: '/openapi.yaml'") {
			t.Fatalf("docs html missing openapi url: %s", body)
		}
	})

	t.Run("serves openapi yaml", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if got := w.Header().Get("Content-Type"); !strings.Contains(got, "application/yaml") {
			t.Fatalf("expected yaml content type, got %q", got)
		}
		body := w.Body.String()
		if !strings.Contains(body, "swagger:") {
			t.Fatalf("unexpected openapi body: %s", w.Body.String())
		}
		if !strings.Contains(body, "/api/v1/access/keypairs:") {
			t.Fatalf("generated swagger is missing expected path definitions: %s", body)
		}
	})
}

func TestNewRouterHidesDocsInReleaseMode(t *testing.T) {
	setGinMode(t, gin.ReleaseMode)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Compute: &compute.Handler{},
	})

	for _, path := range []string{"/docs", "/openapi.yaml"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected %s to return 404 in release mode, got %d", path, w.Code)
		}
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
