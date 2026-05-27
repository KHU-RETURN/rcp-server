package server

import (
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	envAllowedOrigins       = "RCP_ALLOWED_ORIGINS"
	envAllowedOriginPattern = "RCP_ALLOWED_ORIGIN_PATTERN"
)

func corsMiddleware() gin.HandlerFunc {
	allowedOrigins := parseAllowedOrigins(os.Getenv(envAllowedOrigins))
	allowedOriginPattern := strings.TrimSpace(os.Getenv(envAllowedOriginPattern))

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" && isOriginAllowed(origin, allowedOrigins, allowedOriginPattern) {
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

func isOriginAllowed(origin string, allowedOrigins map[string]bool, allowedOriginPattern string) bool {
	if allowedOrigins[origin] {
		return true
	}
	if allowedOriginPattern == "" {
		return false
	}

	matched, err := regexp.MatchString(allowedOriginPattern, origin)
	return err == nil && matched
}
