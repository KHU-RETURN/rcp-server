package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/utils"
)

type Config struct {
	Listen       string        // outer SSH listen address, e.g. ":2222"
	HostKeyPath  string        // ed25519 host key file (created on first boot)
	NotifySock   string        // Unix socket the API posts notifications to
	NotifySecret string        // HMAC-SHA256 shared secret with the API
	NsProxySock  string        // ns-proxy Unix socket
	AuthURLBase  string        // base URL printed in keyboard-interactive ("https://rcp.return.dev")
	NonceTTL     time.Duration // pending-session lifetime
	DBPath       string        // ent SQLite path (read access)
	LogLevel     string
}

func LoadConfig(getenv func(string) string) (*Config, error) {
	// Bind to localhost by default. External exposure is expected to go
	// through cloudflared (or another tunnel running on the same host) so the
	// SSH port is never reachable from the internet directly. To bind to all
	// interfaces, set RCP_SSH_GW_LISTEN=:2222 explicitly.
	listen := strings.TrimSpace(getenv("RCP_SSH_GW_LISTEN"))
	if listen == "" {
		listen = "127.0.0.1:2222"
	}

	hostKey, err := utils.EnvSockPath(getenv, "RCP_SSH_GW_HOST_KEY_PATH", "/etc/rcp/ssh-gateway/host_ed25519")
	if err != nil {
		return nil, err
	}
	notifySock, err := utils.EnvSockPath(getenv, "RCP_SSH_GW_NOTIFY_SOCK", "/run/rcp/ssh-gateway-notify.sock")
	if err != nil {
		return nil, err
	}
	nsProxySock, err := utils.EnvSockPath(getenv, "RCP_NS_PROXY_SOCK", "/run/rcp/ns-proxy.sock")
	if err != nil {
		return nil, err
	}

	notifySecret := strings.TrimSpace(getenv("RCP_SSH_GW_NOTIFY_SECRET"))
	if notifySecret == "" {
		return nil, fmt.Errorf("RCP_SSH_GW_NOTIFY_SECRET: required")
	}

	authURL := strings.TrimSpace(getenv("RCP_SSH_GW_AUTH_URL_BASE"))
	if authURL == "" {
		return nil, fmt.Errorf("RCP_SSH_GW_AUTH_URL_BASE: required")
	}

	dbPath := strings.TrimSpace(getenv("RCP_SSH_GW_DB_PATH"))
	if dbPath == "" {
		return nil, fmt.Errorf("RCP_SSH_GW_DB_PATH: required")
	}

	ttl, err := utils.EnvPositiveDuration(getenv, "RCP_SSH_GW_NONCE_TTL", 5*time.Minute)
	if err != nil {
		return nil, err
	}

	logLevel, err := utils.EnvLogLevel(getenv, "RCP_SSH_GW_LOG_LEVEL", "info")
	if err != nil {
		return nil, err
	}

	return &Config{
		Listen:       listen,
		HostKeyPath:  hostKey,
		NotifySock:   notifySock,
		NotifySecret: notifySecret,
		NsProxySock:  nsProxySock,
		AuthURLBase:  authURL,
		NonceTTL:     ttl,
		DBPath:       dbPath,
		LogLevel:     logLevel,
	}, nil
}
