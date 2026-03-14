package server

import (
	"github.com/gin-gonic/gin"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
)

func NewRouter(app *App) *gin.Engine {
	r := gin.Default()

	v1 := r.Group("/api/v1")
	{
		v1.GET("/flavors", computeHandler.GetFlavors)
	}

	return r
}