package server

import (
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/api"
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
	var foundConsoleWebSocket bool
	var foundAuthorizedKeys bool

	for _, route := range routes {
		if route.Method == "GET" && route.Path == api.BasePath+"/auth/oauth/google" {
			foundAuthLogin = true
		}
		if route.Method == "GET" && route.Path == api.BasePath+"/compute/flavors" {
			foundFlavors = true
		}
		if route.Method == "POST" && route.Path == api.BasePath+"/compute/instances" {
			foundCreateInstance = true
		}
		if route.Method == "GET" && route.Path == api.BasePath+"/access/console/ws" {
			foundConsoleWebSocket = true
		}
		if route.Method == "GET" && route.Path == api.BasePath+"/internal/ssh/authorized-keys" {
			foundAuthorizedKeys = true
		}
	}

	if !foundFlavors {
		t.Fatalf("GET %s/compute/flavors route was not registered", api.BasePath)
	}
	if !foundCreateInstance {
		t.Fatalf("POST %s/compute/instances route was not registered", api.BasePath)
	}
	if !foundAuthLogin {
		t.Fatalf("GET %s/auth/oauth/google route was not registered", api.BasePath)
	}
	if !foundConsoleWebSocket {
		t.Fatalf("GET %s/access/console/ws route was not registered", api.BasePath)
	}
	if !foundAuthorizedKeys {
		t.Fatalf("GET %s/internal/ssh/authorized-keys route was not registered", api.BasePath)
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
