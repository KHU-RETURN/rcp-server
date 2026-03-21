package server

import (
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/gin-gonic/gin"
)

func TestNewRouterRegistersComputeRoutes(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	router := NewRouter(&App{
		Compute: &compute.Handler{},
	})

	routes := router.Routes()
	var foundFlavors bool
	var foundCreateInstance bool

	for _, route := range routes {
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
}
