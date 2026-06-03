package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/utils"
)

type Config struct {
	Listen             string        // outer SSH listen address, e.g. ":2222"
	HostKeyPath        string        // ed25519 host key file (created on first boot)
	KnownHostsPath     string        // trusted inner VM host keys
	NotifySock         string        // Unix socket the API posts notifications to
	NotifySecret       string        // HMAC-SHA256 shared secret with the API
	NsProxySock        string        // ns-proxy Unix socket
	AuthURLBase        string        // base URL printed in keyboard-interactive ("https://rcp.return.dev")
	APIURLBase         string        // API origin used for internal ephemeral-key registration
	NonceTTL           time.Duration // pending-session lifetime
	MaxPendingSessions int           // pending OAuth sessions cap
	FixedNetworkName   string        // optional OpenStack network name for fixed IP selection
	VMUsers            []string      // inner SSH login users tried in order
	DBDriver           string        // ent DB driver
	DBDSN              string        // ent DB DSN (opened without migration)
	DBPath             string        // legacy sqlite path used when DB_DSN is unset
	LogLevel           string
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
	knownHosts, err := utils.EnvSockPath(getenv, "RCP_SSH_GW_KNOWN_HOSTS_PATH", "/etc/rcp/ssh-gateway/known_hosts")
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

	authURL := strings.TrimRight(strings.TrimSpace(getenv("RCP_SSH_GW_AUTH_URL_BASE")), "/")
	if authURL == "" {
		return nil, fmt.Errorf("RCP_SSH_GW_AUTH_URL_BASE: required")
	}
	apiURL := strings.TrimRight(strings.TrimSpace(getenv("RCP_SSH_GW_API_URL_BASE")), "/")
	if apiURL == "" {
		return nil, fmt.Errorf("RCP_SSH_GW_API_URL_BASE: required")
	}

	dbDriver := strings.TrimSpace(getenv("DB_DRIVER"))
	if dbDriver == "" {
		dbDriver = "sqlite3"
	}
	dbDSN := strings.TrimSpace(getenv("DB_DSN"))
	dbPath := ""
	if dbDSN == "" {
		var err error
		if strings.TrimSpace(getenv("RCP_SSH_GW_DB_PATH")) == "" {
			return nil, fmt.Errorf("RCP_SSH_GW_DB_PATH or DB_DSN: required")
		}
		dbPath, err = utils.EnvSockPath(getenv, "RCP_SSH_GW_DB_PATH", "")
		if err != nil {
			return nil, err
		}
		dbDSN = "file:" + dbPath + "?mode=ro&cache=shared&_pragma=foreign_keys(1)"
	}
	if err := validateGatewayDBDSN(dbDriver, dbDSN); err != nil {
		return nil, err
	}

	ttl, err := utils.EnvPositiveDuration(getenv, "RCP_SSH_GW_NONCE_TTL", 5*time.Minute)
	if err != nil {
		return nil, err
	}
	maxPending, err := utils.EnvInt(getenv, "RCP_SSH_GW_MAX_PENDING_SESSIONS", defaultMaxPendingSessions)
	if err != nil {
		return nil, err
	}
	if maxPending <= 0 {
		return nil, fmt.Errorf("RCP_SSH_GW_MAX_PENDING_SESSIONS: must be > 0, got %d", maxPending)
	}
	fixedNetwork := strings.TrimSpace(getenv("RCP_SSH_GW_FIXED_NETWORK"))
	vmUsers := parseVMUsers(getenv)

	logLevel, err := utils.EnvLogLevel(getenv, "RCP_SSH_GW_LOG_LEVEL", "info")
	if err != nil {
		return nil, err
	}

	return &Config{
		Listen:             listen,
		HostKeyPath:        hostKey,
		KnownHostsPath:     knownHosts,
		NotifySock:         notifySock,
		NotifySecret:       notifySecret,
		NsProxySock:        nsProxySock,
		AuthURLBase:        authURL,
		APIURLBase:         apiURL,
		NonceTTL:           ttl,
		MaxPendingSessions: maxPending,
		FixedNetworkName:   fixedNetwork,
		VMUsers:            vmUsers,
		DBDriver:           dbDriver,
		DBDSN:              dbDSN,
		DBPath:             dbPath,
		LogLevel:           logLevel,
	}, nil
}

func parseVMUsers(getenv func(string) string) []string {
	raw := strings.TrimSpace(getenv("RCP_SSH_GW_VM_USERS"))
	if raw == "" {
		raw = "ubuntu,rocky"
	}
	seen := make(map[string]bool)
	var users []string
	for _, part := range strings.Split(raw, ",") {
		user := strings.TrimSpace(part)
		if user == "" || seen[user] {
			continue
		}
		seen[user] = true
		users = append(users, user)
	}
	if len(users) == 0 {
		return []string{"ubuntu", "rocky"}
	}
	return users
}

func validateGatewayDBDSN(driver, dsn string) error {
	if !strings.EqualFold(strings.TrimSpace(driver), "sqlite3") {
		return nil
	}

	clean := strings.TrimSpace(dsn)
	path := clean
	if strings.HasPrefix(clean, "file:") {
		path, _, _ = strings.Cut(strings.TrimPrefix(clean, "file:"), "?")
	}
	if filepath.IsAbs(path) {
		return nil
	}
	return fmt.Errorf("DB_DSN: sqlite3 DSN must use an absolute path, got %q", dsn)
}
