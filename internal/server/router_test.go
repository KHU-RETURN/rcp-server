package server

import (
	"net/http"
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
	if !foundConsoleWebSocket {
		t.Fatalf("%s %s/access/console/ws route was not registered", http.MethodGet, api.BasePath)
	}
	if !foundAuthorizedKeys {
		t.Fatalf("%s %s/internal/ssh/authorized-keys route was not registered", http.MethodGet, api.BasePath)
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
