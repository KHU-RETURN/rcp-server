package access

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

const (
	routeAccessPrefix      = "/access"
	routeInternalSSHPrefix = "/internal/ssh"

	pathConsoleWebSocket = "/console/ws"
	pathConsoleSessions  = "/instances/:id/console-sessions"
	pathAuthorizedKeys   = "/authorized-keys"

	envWebConsoleBaseURL        = "RCP_WEB_CONSOLE_BASE_URL"
	envWebConsoleAllowedOrigins = "RCP_WEB_CONSOLE_ALLOWED_ORIGINS"
	envNSProxySock              = "RCP_NS_PROXY_SOCK"

	defaultNSProxySock = "/run/rcp/ns-proxy.sock"
)

type Handler struct {
	Svc             *Service
	NSProxySockPath string
}

func NewHandler(svc *Service) *Handler {
	return &Handler{
		Svc:             svc,
		NSProxySockPath: defaultNSProxySockPath(),
	}
}

func (h *Handler) InitPublicRoutes(rg *gin.RouterGroup) {
	accessGroup := rg.Group(routeAccessPrefix)
	{
		accessGroup.GET(pathConsoleWebSocket, h.WebConsole)
	}
}

func (h *Handler) InitInternalRoutes(rg *gin.RouterGroup) {
	internalGroup := rg.Group(routeInternalSSHPrefix)
	{
		internalGroup.GET(pathAuthorizedKeys, h.GetAuthorizedKeys)
	}
}

func (h *Handler) InitRoutes(rg *gin.RouterGroup) {
	accessGroup := rg.Group(routeAccessPrefix)
	{
		accessGroup.GET("/keypairs", h.ListKeyPairs)
		accessGroup.POST("/keypairs", h.CreateKeyPair)
		accessGroup.GET("/keypairs/:name", h.GetKeyPair)
		accessGroup.DELETE("/keypairs/:name", h.DeleteKeyPair)
		// PUT/PATCH 미제공: SSH 키페어는 수정이 불가능하며, 변경 시 삭제 후 재생성이 표준입니다.
		accessGroup.POST(pathConsoleSessions, h.CreateConsoleSession)
	}
}

func (h *Handler) ListKeyPairs(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	kps, err := h.Svc.ListKeyPairs(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, kps)
}

func (h *Handler) GetKeyPair(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	name := c.Param("name")
	kp, err := h.Svc.GetKeyPair(c.Request.Context(), id, name)
	if err != nil {
		if errors.Is(err, ErrKeyPairNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, kp)
}

func (h *Handler) DeleteKeyPair(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	name := c.Param("name")
	if err := h.Svc.DeleteKeyPair(c.Request.Context(), id, name); err != nil {
		if errors.Is(err, ErrKeyPairNotFound) {
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) CreateKeyPair(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req CreateKeyPairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		return
	}

	res, err := h.Svc.CreateKeyPair(c.Request.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNameRequired), errors.Is(err, ErrPublicKeyRequired), errors.Is(err, ErrInvalidSSHKeyFormat):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrInvalidKeyPairRequest):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid keypair request"})
		case errors.Is(err, ErrKeyPairAccessDenied):
			c.JSON(http.StatusForbidden, api.ErrorResponse{Error: "keypair access denied"})
		case errors.Is(err, ErrKeyPairAlreadyExists):
			c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrKeyPairOperationFailed):
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to create keypair"})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "internal server error"})
		}
		return
	}

	c.JSON(http.StatusCreated, res)
}

func (h *Handler) CreateConsoleSession(c *gin.Context) {
	id, ok := api.OwnerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req CreateConsoleSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		return
	}

	res, err := h.Svc.CreateConsoleSession(c.Request.Context(), id, c.Param("id"), req, websocketBaseURL(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrConsoleInstanceIDRequired):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrConsoleInstanceNotFound):
			c.JSON(http.StatusNotFound, api.ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrConsoleInstanceNoIP):
			c.JSON(http.StatusConflict, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to create console session"})
		}
		return
	}

	c.JSON(http.StatusCreated, res)
}

func (h *Handler) GetAuthorizedKeys(c *gin.Context) {
	instanceID := c.Query("instance_id")
	username := c.Query("user")
	keys := h.Svc.AuthorizedKeys(instanceID, username)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(keys))
}

func (h *Handler) WebConsole(c *gin.Context) {
	token := c.Query("token")
	session, ok := h.Svc.TakeConsoleSession(token)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "invalid console token"})
		return
	}

	server := websocket.Server{
		Handshake: validateWebSocketOrigin,
		Handler: func(ws *websocket.Conn) {
			defer func() { _ = ws.Close() }()
			_ = h.bridgeWebConsole(ws, session)
		},
	}
	server.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) bridgeWebConsole(ws *websocket.Conn, console *consoleSession) error {
	tcpConn, err := dialViaNSProxy(h.NSProxySockPath, console.Host+":22")
	if err != nil {
		return err
	}
	defer func() { _ = tcpConn.Close() }()

	sshCfg := &ssh.ClientConfig{
		User: console.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(console.Signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106 -- web console uses short-lived keys for tenant VMs without managed host key inventory.
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, console.Host+":22", sshCfg)
	if err != nil {
		return err
	}
	h.Svc.DeleteAuthorizedKey(console.InstanceID, console.Username, console.AuthorizedKey)
	client := ssh.NewClient(sshConn, chans, reqs)
	defer func() { _ = client.Close() }()

	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = sess.Close() }()

	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return err
	}

	if err := sess.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{}); err != nil {
		return err
	}
	if err := sess.Shell(); err != nil {
		return err
	}

	var writeMu sync.Mutex
	done := make(chan struct{})
	go copySSHToWebSocket(ws, stdout, &writeMu, done)
	go copySSHToWebSocket(ws, stderr, &writeMu, done)
	go copyWebSocketToSSH(ws, stdin, done)

	err = sess.Wait()
	h.Svc.DeleteAuthorizedKey(console.InstanceID, console.Username, console.AuthorizedKey)
	close(done)
	return err
}

func copySSHToWebSocket(ws *websocket.Conn, r io.Reader, mu *sync.Mutex, done <-chan struct{}) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			payload := append([]byte(nil), buf[:n]...)
			mu.Lock()
			_ = websocket.Message.Send(ws, payload)
			mu.Unlock()
		}
		if err != nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func copyWebSocketToSSH(ws *websocket.Conn, w io.WriteCloser, done <-chan struct{}) {
	defer func() { _ = w.Close() }()

	for {
		var payload []byte
		if err := websocket.Message.Receive(ws, &payload); err != nil {
			return
		}
		if len(payload) > 0 {
			_, _ = w.Write(payload)
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func websocketBaseURL(c *gin.Context) string {
	if baseURL := configuredWebConsoleBaseURL(); baseURL != "" {
		return baseURL + api.BasePath
	}

	scheme := "ws"
	if c.Request.TLS != nil {
		scheme = "wss"
	}
	return scheme + "://" + c.Request.Host + api.BasePath
}

func configuredWebConsoleBaseURL() string {
	rawURL := strings.TrimSpace(os.Getenv(envWebConsoleBaseURL))
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(rawURL, "/")
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func validateWebSocketOrigin(config *websocket.Config, req *http.Request) error {
	if isWebSocketOriginAllowed(config, req) {
		return nil
	}
	return errors.New("websocket origin is not allowed")
}

func isWebSocketOriginAllowed(config *websocket.Config, req *http.Request) bool {
	origin, ok := websocketOrigin(config, req)
	if !ok {
		return true
	}
	if origin == nil {
		return false
	}

	if allowedOrigins := configuredWebConsoleAllowedOrigins(); len(allowedOrigins) > 0 {
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin.String() {
				return true
			}
		}
		return false
	}

	return sameHost(origin.Host, req.Host)
}

func websocketOrigin(config *websocket.Config, req *http.Request) (*url.URL, bool) {
	if config != nil && config.Origin != nil {
		return normalizeOrigin(config.Origin), true
	}

	rawOrigin := strings.TrimSpace(req.Header.Get("Origin"))
	if rawOrigin == "" {
		return nil, false
	}
	origin, err := url.Parse(rawOrigin)
	if err != nil {
		return nil, true
	}
	return normalizeOrigin(origin), true
}

func normalizeOrigin(origin *url.URL) *url.URL {
	if origin == nil || origin.Scheme == "" || origin.Host == "" {
		return nil
	}
	return &url.URL{
		Scheme: strings.ToLower(origin.Scheme),
		Host:   strings.ToLower(origin.Host),
	}
}

func configuredWebConsoleAllowedOrigins() []string {
	rawOrigins := strings.TrimSpace(os.Getenv(envWebConsoleAllowedOrigins))
	if rawOrigins == "" {
		return nil
	}

	origins := strings.Split(rawOrigins, ",")
	allowedOrigins := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			allowedOrigins = append(allowedOrigins, origin)
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil {
			continue
		}
		normalized := normalizeOrigin(parsed)
		if normalized == nil {
			continue
		}
		allowedOrigins = append(allowedOrigins, normalized.String())
	}
	return allowedOrigins
}

func sameHost(left, right string) bool {
	leftHost, leftPort, leftHasPort := splitHostPort(left)
	rightHost, rightPort, rightHasPort := splitHostPort(right)
	if leftHost != rightHost {
		return false
	}
	if leftHasPort != rightHasPort {
		return false
	}
	return !leftHasPort || leftPort == rightPort
}

func splitHostPort(host string) (string, string, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", "", false
	}

	parsedHost, parsedPort, err := net.SplitHostPort(host)
	if err == nil {
		return parsedHost, parsedPort, true
	}

	return host, "", false
}

func defaultNSProxySockPath() string {
	if path := os.Getenv(envNSProxySock); path != "" {
		return path
	}
	return defaultNSProxySock
}
