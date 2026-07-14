package server

import (
	"log"
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
	allowedOriginPattern := compileAllowedOriginPattern(os.Getenv(envAllowedOriginPattern))

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

// compileAllowedOriginPattern compiles the pattern once at middleware setup
// instead of on every request. An invalid pattern is logged and treated as
// unset rather than failing startup.
func compileAllowedOriginPattern(raw string) *regexp.Regexp {
	pattern := strings.TrimSpace(raw)
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		log.Printf("invalid %s: %v", envAllowedOriginPattern, err)
		return nil
	}
	return re
}

func isOriginAllowed(origin string, allowedOrigins map[string]bool, allowedOriginPattern *regexp.Regexp) bool {
	if allowedOrigins[origin] {
		return true
	}
	if allowedOriginPattern == nil {
		return false
	}
	return allowedOriginPattern.MatchString(origin)
}
