package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/KHU-RETURN/rcp-server/internal/api"
)

func NewRouter(app *App) *gin.Engine {
	r := gin.Default()
	r.Use(corsMiddleware())
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	v1 := r.Group(api.BasePath)
	{
		app.Auth.InitRoutes(v1)
		app.Access.InitPublicRoutes(v1)
		app.Access.InitInternalRoutes(v1)

		protected := v1.Group("/")
		if app.Auth != nil {
			protected.Use(app.Auth.AuthRequired())
		}

		app.Access.InitRoutes(protected)
		app.Compute.InitRoutes(protected)
		app.Storage.InitRoutes(protected)
	}

	return r
}
