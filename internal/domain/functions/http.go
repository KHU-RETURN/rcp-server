package functions

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HTTPEvent is the JSON object passed to a public function on stdin.
type HTTPEvent struct {
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	Query           string            `json:"query"`
	Headers         map[string]string `json:"headers"`
	Body            string            `json:"body"`
	IsBase64Encoded bool              `json:"isBase64Encoded"`
}

// HTTPResponse is the JSON object a function writes to stdout.
type HTTPResponse struct {
	StatusCode      int               `json:"statusCode"`
	Headers         map[string]string `json:"headers"`
	Body            string            `json:"body"`
	IsBase64Encoded bool              `json:"isBase64Encoded"`
}

var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func (h *Handler) InitPublicRoutes(r *gin.RouterGroup) {
	r.Match([]string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}, "/run/:id/*path", h.publicHTTP)
}

func (h *Handler) publicHTTP(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
	if c.Request.Method == http.MethodOptions {
		c.Status(http.StatusNoContent)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, api.ErrorResponse{Error: "function not found"})
		return
	}
	auth := strings.Fields(c.GetHeader("Authorization"))
	if len(auth) != 2 || !strings.EqualFold(auth[0], "Bearer") {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: ErrInvalidKey.Error()})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxHTTPBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, api.ErrorResponse{Error: "request body exceeds 65536 bytes"})
		return
	}
	event := newHTTPEvent(c.Request, c.Param("path"), body)
	input, err := json.Marshal(event)
	if err != nil || len(input) > MaxInputBytes {
		c.JSON(http.StatusRequestEntityTooLarge, api.ErrorResponse{Error: "request metadata exceeds limit"})
		return
	}
	result, err := h.Svc.InvokePublic(c.Request.Context(), id, auth[1], input)
	if err != nil {
		writeError(c, err)
		return
	}
	if result.ExitCode != 0 {
		c.JSON(http.StatusBadGateway, api.ErrorResponse{Error: "function exited with an error"})
		return
	}
	var response HTTPResponse
	if err := json.Unmarshal([]byte(result.Stdout), &response); err != nil || response.StatusCode < 200 || response.StatusCode > 599 {
		c.JSON(http.StatusBadGateway, api.ErrorResponse{Error: "invalid function HTTP response"})
		return
	}
	output := []byte(response.Body)
	if response.IsBase64Encoded {
		output, err = base64.StdEncoding.DecodeString(response.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, api.ErrorResponse{Error: "invalid function response body"})
			return
		}
	}
	if len(output) > MaxHTTPBodyBytes {
		c.JSON(http.StatusBadGateway, api.ErrorResponse{Error: "function response body exceeds limit"})
		return
	}
	for name, value := range response.Headers {
		if safeResponseHeader(name, value) {
			c.Header(name, value)
		}
	}
	c.Status(response.StatusCode)
	_, _ = c.Writer.Write(output)
}

func newHTTPEvent(request *http.Request, path string, body []byte) HTTPEvent {
	if path == "" {
		path = "/"
	}
	event := HTTPEvent{Method: request.Method, Path: path, Query: request.URL.RawQuery, Headers: make(map[string]string), Body: string(body)}
	if !utf8.Valid(body) {
		event.Body, event.IsBase64Encoded = base64.StdEncoding.EncodeToString(body), true
	}
	for name, values := range request.Header {
		canonical := http.CanonicalHeaderKey(name)
		if sensitiveRequestHeader(canonical) {
			continue
		}
		event.Headers[canonical] = strings.Join(values, ", ")
	}
	return event
}

func sensitiveRequestHeader(name string) bool {
	return name == "Cookie" || name == "Authorization" || name == "Proxy-Authorization" ||
		strings.HasPrefix(name, "X-Forwarded-") || strings.HasPrefix(name, "Cf-") || strings.HasPrefix(name, "X-Rcp-")
}

func safeResponseHeader(name, value string) bool {
	canonical := http.CanonicalHeaderKey(name)
	if !headerName.MatchString(name) || strings.ContainsAny(value, "\r\n") {
		return false
	}
	switch canonical {
	case "Set-Cookie", "Content-Length", "Transfer-Encoding", "Connection", "Keep-Alive", "Upgrade", "Server", "Proxy-Authenticate", "Proxy-Authorization":
		return false
	}
	return !strings.HasPrefix(canonical, "X-Rcp-")
}
