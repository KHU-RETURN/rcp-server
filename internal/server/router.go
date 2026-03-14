package server

import (
	"github.com/gin-gonic/gin"
)

func NewRouter(app *App) *gin.Engine {
	r := gin.Default()

	v1 := r.Group("/api/v1")
	{
		app.Compute.InitRoutes(v1)
	}

	return r
}