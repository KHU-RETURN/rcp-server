package server

import (
	_ "embed"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
)

//go:embed scalar.html
var scalarDocsHTML []byte

func registerDocsRoutes(r *gin.Engine) {
	r.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", scalarDocsHTML)
	})

	r.GET("/openapi.yaml", func(c *gin.Context) {
		spec, err := os.ReadFile(openAPISpecPath())
		if err != nil {
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to load local OpenAPI spec"})
			return
		}

		c.Data(http.StatusOK, "application/yaml; charset=utf-8", spec)
	})
}

func openAPISpecPath() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "openapi.yaml"
	}

	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "openapi.yaml"))
}
