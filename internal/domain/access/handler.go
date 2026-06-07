package access

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/KHU-RETURN/rcp-server/internal/api"
)

const (
	webConsoleDialTimeout  = 15 * time.Second
	webConsoleWriteTimeout = 10 * time.Second
	webConsoleReadLimit    = 1 << 20
)

const (
	routeAccessPrefix      = "/access"
	routeInternalSSHPrefix = "/internal/ssh"

	pathConsoleWebSocket = "/console/ws"
	pathConsoleSessions  = "/instances/:id/console-sessions"
	pathAuthorizedKeys   = "/authorized-keys"
	pathEphemeralKeys    = "/ephemeral-keys"

	envWebConsoleBaseURL        = "RCP_WEB_CONSOLE_BASE_URL"
	envWebConsoleAllowedOrigins = "RCP_WEB_CONSOLE_ALLOWED_ORIGINS"
	envNSProxySock              = "RCP_NS_PROXY_SOCK"
	envVMKnownHostsPath         = "RCP_VM_KNOWN_HOSTS_PATH"
	envSSHGatewayKnownHostsPath = "RCP_SSH_GW_KNOWN_HOSTS_PATH"

	defaultNSProxySock      = "/run/rcp/ns-proxy.sock"
	defaultVMKnownHostsPath = "/etc/rcp/ssh-gateway/known_hosts"
)

type Handler struct {
	Svc               *Service
	NSProxySockPath   string
	VMHostKeyCallback ssh.HostKeyCallback
	InternalSecret    []byte
}

func NewHandler(svc *Service) *Handler {
	return &Handler{
		Svc:               svc,
		NSProxySockPath:   defaultNSProxySockPath(),
		VMHostKeyCallback: defaultVMHostKeyCallback(),
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
		internalGroup.POST(pathEphemeralKeys, h.AddEphemeralAuthorizedKey)
		internalGroup.DELETE(pathEphemeralKeys, h.DeleteEphemeralAuthorizedKey)
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

func (h *Handler) AddEphemeralAuthorizedKey(c *gin.Context) {
	var req EphemeralAuthorizedKeyRequest
	if !h.readSignedJSON(c, &req) {
		return
	}
	if err := h.Svc.AddEphemeralAuthorizedKey(req); err != nil {
		switch {
		case errors.Is(err, ErrConsoleInstanceIDRequired), errors.Is(err, ErrPublicKeyRequired), errors.Is(err, ErrInvalidSSHKeyFormat):
			c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to register ephemeral key"})
		}
		return
	}
	c.Status(http.StatusCreated)
}

func (h *Handler) DeleteEphemeralAuthorizedKey(c *gin.Context) {
	var req EphemeralAuthorizedKeyRequest
	if !h.readSignedJSON(c, &req) {
		return
	}
	h.Svc.DeleteEphemeralAuthorizedKey(req)
	c.Status(http.StatusNoContent)
}

func (h *Handler) readSignedJSON(c *gin.Context, out any) bool {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<16))
	if err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "failed to read request body"})
		return false
	}
	if !verifyInternalHMAC(h.InternalSecret, body, c.GetHeader(InternalSigHeader)) {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "invalid signature"})
		return false
	}
	if err := json.Unmarshal(body, out); err != nil {
		c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body"})
		return false
	}
	return true
}

func verifyInternalHMAC(secret, body []byte, gotHex string) bool {
	if len(secret) == 0 {
		return false
	}
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}

func (h *Handler) WebConsole(c *gin.Context) {
	token := c.Query("token")
	session, ok := h.Svc.TakeConsoleSession(token)
	if !ok {
		c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "invalid console token"})
		return
	}

	if !isWebSocketOriginAllowed(c.Request) {
		c.JSON(http.StatusForbidden, api.ErrorResponse{Error: "websocket origin is not allowed"})
		return
	}

	// Origin is validated above; skip the library's built-in check.
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Default().Warn("web console websocket accept failed",
			"instance_id", session.InstanceID,
			"err", err,
		)
		return
	}
	conn.SetReadLimit(webConsoleReadLimit)
	defer func() { _ = conn.CloseNow() }()

	if err := h.relayConsole(newWebSocketConsole(conn), session); err != nil {
		slog.Default().Warn("web console bridge failed",
			"instance_id", session.InstanceID,
			"host", session.Host,
			"username", session.Username,
			"err", err,
		)
	}
}

// relayConsole runs an SSH shell over a console session against any client transport.
func (h *Handler) relayConsole(client io.ReadWriteCloser, console *consoleSession) error {
	sshAddress := net.JoinHostPort(console.Host, "22")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialCtx, dialCancel := context.WithTimeout(ctx, webConsoleDialTimeout)
	tcpConn, err := dialViaNSProxy(dialCtx, h.NSProxySockPath, sshAddress)
	dialCancel()
	if err != nil {
		return err
	}
	defer func() { _ = tcpConn.Close() }()

	sshCfg := &ssh.ClientConfig{
		User: console.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(console.Signer),
		},
		HostKeyCallback: vmHostKeyCallbackForAddress(sshAddress, h.VMHostKeyCallback),
		Timeout:         webConsoleDialTimeout,
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, sshAddress, sshCfg)
	if err != nil {
		return err
	}
	h.Svc.DeleteAuthorizedKey(console.InstanceID, console.Username, console.AuthorizedKey)
	sshClient := ssh.NewClient(sshConn, chans, reqs)
	defer func() { _ = sshClient.Close() }()

	sess, err := sshClient.NewSession()
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

	go func() { defer cancel(); _, _ = io.Copy(client, stdout) }()
	go func() { defer cancel(); _, _ = io.Copy(client, stderr) }()
	go func() { defer cancel(); _, _ = io.Copy(stdin, client); _ = stdin.Close() }()

	waitErr := make(chan error, 1)
	go func() { waitErr <- sess.Wait() }()

	select {
	case err = <-waitErr:
	case <-ctx.Done():
		_ = sess.Close()
		err = <-waitErr
	}
	// Closing the transport unblocks the input pump still reading from the client.
	_ = client.Close()
	h.Svc.DeleteAuthorizedKey(console.InstanceID, console.Username, console.AuthorizedKey)
	return err
}

func defaultVMHostKeyCallback() ssh.HostKeyCallback {
	return reloadingVMHostKeyCallback(configuredVMKnownHostsPath())
}

func reloadingVMHostKeyCallback(path string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		cb, err := loadVMHostKeyCallback(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return trustVMHostKeyOnFirstUse(path, hostname, key)
			}
			return fmt.Errorf("vm host key trust unavailable at %s: %w", path, err)
		}
		if err := cb(hostname, remote, key); err != nil {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
				return trustVMHostKeyOnFirstUse(path, hostname, key)
			}
			return err
		}
		return nil
	}
}

func trustVMHostKeyOnFirstUse(path, hostname string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("prepare vm host key trust store %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640) //nolint:gosec // operator-controlled trust store path
	if err != nil {
		return fmt.Errorf("open vm host key trust store %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("append vm host key trust store %s: %w", path, err)
	}
	return nil
}

func vmHostKeyCallbackForAddress(address string, cb ssh.HostKeyCallback) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil {
			return err
		}
		return cb(address, &net.TCPAddr{IP: net.ParseIP(host), Port: portNumber}, key)
	}
}

func configuredVMKnownHostsPath() string {
	if v := strings.TrimSpace(os.Getenv(envVMKnownHostsPath)); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(envSSHGatewayKnownHostsPath)); v != "" {
		return v
	}
	return defaultVMKnownHostsPath
}

func loadVMHostKeyCallback(path string) (ssh.HostKeyCallback, error) {
	clean := strings.TrimSpace(path)
	if clean == "" {
		clean = defaultVMKnownHostsPath
	}
	if !filepath.IsAbs(clean) {
		return nil, fmt.Errorf("known_hosts path must be absolute: %q", clean)
	}
	return knownhosts.New(clean)
}

// webSocketConsole adapts a WebSocket to the io.ReadWriteCloser the relay drives.
type webSocketConsole struct {
	conn *websocket.Conn
	mu   sync.Mutex
	rest []byte
}

func newWebSocketConsole(conn *websocket.Conn) *webSocketConsole {
	return &webSocketConsole{conn: conn}
}

func (w *webSocketConsole) Read(p []byte) (int, error) {
	if len(w.rest) == 0 {
		_, data, err := w.conn.Read(context.Background())
		if err != nil {
			return 0, err
		}
		w.rest = data
	}
	n := copy(p, w.rest)
	w.rest = w.rest[n:]
	return n, nil
}

func (w *webSocketConsole) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), webConsoleWriteTimeout)
	defer cancel()
	if err := w.conn.Write(ctx, websocket.MessageBinary, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *webSocketConsole) Close() error {
	return w.conn.Close(websocket.StatusNormalClosure, "")
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

func isWebSocketOriginAllowed(req *http.Request) bool {
	origin, ok := websocketOrigin(req)
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

func websocketOrigin(req *http.Request) (*url.URL, bool) {
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
