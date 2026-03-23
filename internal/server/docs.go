package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gin-gonic/gin"
)

const scalarDocsHTML = `<!doctype html>
<html lang="ko">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>RCP API Docs</title>
  </head>
  <body>
    <div id="app"></div>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
      Scalar.createApiReference('#app', {
        url: '/openapi.yaml',
        layout: 'modern',
        agent: {
          disabled: true,
        },
        metaData: {
          title: 'RCP API Docs',
          description: 'Local API reference for RCP server development',
        },
      })
    </script>
  </body>
</html>
`

func registerDocsRoutes(r *gin.Engine) {
	r.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(scalarDocsHTML))
	})

	r.GET("/openapi.yaml", func(c *gin.Context) {
		spec, err := os.ReadFile(openAPISpecPath())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load local OpenAPI spec"})
			return
		}

		c.Data(http.StatusOK, "application/yaml; charset=utf-8", spec)
	})
}

func openAPISpecPath() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("docs", "openapi.yaml")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "docs", "openapi.yaml"))
}
