package server

import (
	"fmt"
	"net/http"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
)

func NewRouter(app *App) *gin.Engine {
	r := gin.Default()
	r.Use(sentrygin.New(sentrygin.Options{Repanic: true}))
	r.Use(sentryServerErrorMiddleware())
	r.Use(corsMiddleware())

	v1 := r.Group(api.BasePath)
	{
		app.Auth.InitRoutes(v1)
		app.Access.InitPublicRoutes(v1)
		app.Access.InitInternalRoutes(v1)
		if app.Functions != nil {
			app.Functions.InitPublicRoutes(v1)
		}

		protected := v1.Group("/")
		if app.Auth != nil {
			protected.Use(app.Auth.AuthRequired())
		}

		app.Access.InitRoutes(protected)
		app.Apps.InitRoutes(protected)
		app.BlockStorage.InitRoutes(protected)
		app.Compute.InitRoutes(protected)
		app.Storage.InitRoutes(protected)
		if app.Operations != nil {
			app.Operations.InitRoutes(protected)
		}
		if app.Functions != nil {
			app.Functions.InitRoutes(protected)
		}

		if app.Auth != nil && app.Admin != nil {
			adminGroup := v1.Group("/")
			adminGroup.Use(app.Auth.AuthRequired(), auth.AdminRequired())
			app.Admin.InitRoutes(adminGroup)
			if app.Operations != nil {
				app.Operations.InitAdminRoutes(adminGroup)
			}
		}
	}

	return r
}

func sentryServerErrorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Status() < http.StatusInternalServerError {
			return
		}
		if hub := sentrygin.GetHubFromContext(c); hub != nil {
			path := c.FullPath()
			if path == "" {
				path = "unmatched route"
			}
			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetLevel(sentry.LevelError)
				hub.CaptureMessage(fmt.Sprintf("%s %s returned %d", c.Request.Method, path, c.Writer.Status()))
			})
		}
	}
}
