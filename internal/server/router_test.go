package server

import (
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/gin-gonic/gin"
)

func TestNewRouterRegistersComputeRoutes(t *testing.T) {
	setGinMode(t, gin.TestMode)

	router := NewRouter(&App{
		Access:  &access.Handler{},
		Auth:    &auth.Handler{},
		Compute: &compute.Handler{},
	})

	routes := router.Routes()
	var foundFlavors bool
	var foundCreateInstance bool
	var foundAuthLogin bool

	for _, route := range routes {
		if route.Method == "GET" && route.Path == "/api/v1/auth/oauth/google" {
			foundAuthLogin = true
		}
		if route.Method == "GET" && route.Path == "/api/v1/compute/flavors" {
			foundFlavors = true
		}
		if route.Method == "POST" && route.Path == "/api/v1/compute/instances" {
			foundCreateInstance = true
		}
	}

	if !foundFlavors {
		t.Fatalf("GET /api/v1/compute/flavors route was not registered")
	}
	if !foundCreateInstance {
		t.Fatalf("POST /api/v1/compute/instances route was not registered")
	}
	if !foundAuthLogin {
		t.Fatalf("GET /api/v1/auth/oauth/google route was not registered")
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
