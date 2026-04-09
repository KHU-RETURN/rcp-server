package server

import (
	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
)

func NewRouter(app *App) *gin.Engine {
	r := gin.Default()

	v1 := r.Group(api.BasePath)
	{
		app.Auth.InitRoutes(v1)

		protected := v1.Group("/")
		if app.Auth != nil {
			protected.Use(app.Auth.AuthRequired())
		}

		app.Access.InitRoutes(protected)
		app.Compute.InitRoutes(protected)
	}

	return r
}
