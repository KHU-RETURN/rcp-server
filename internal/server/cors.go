package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const envAllowedOrigins = "RCP_ALLOWED_ORIGINS"

func corsMiddleware() gin.HandlerFunc {
	allowedOrigins := parseAllowedOrigins(os.Getenv(envAllowedOrigins))

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" && allowedOrigins[origin] {
			header := c.Writer.Header()
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Credentials", "true")
			header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			header.Add("Vary", "Origin")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func parseAllowedOrigins(rawOrigins string) map[string]bool {
	allowedOrigins := map[string]bool{}
	for _, origin := range strings.Split(rawOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		allowedOrigins[origin] = true
	}
	return allowedOrigins
}
