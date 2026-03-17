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
	for _, route := range routes {
		if route.Method == "GET" && route.Path == "/api/v1/compute/flavors" {
			return
		}
	}

	t.Fatalf("GET /api/v1/compute/flavors route was not registered")
}
