package access

import (
	"os"
	"strconv"
	"time"
)

// SSHConfig holds SSH relay server configuration loaded from environment variables.
type SSHConfig struct {
	ListenAddr        string // e.g. ":2222"
	HostKeyPath       string // path to SSH host private key
	CFCAPublicKeyPath string // Cloudflare CA public key for cert verification
	QRouterNamespace  string // qrouter network namespace name
	ServiceKeyPath    string // RCP service private key (ed25519) for VM auth
	MenuPageSize      int    // items per page in interactive VM menu
}

// SSHConfigFromEnv builds an SSHConfig from environment variables.
func SSHConfigFromEnv() SSHConfig {
	pageSize := 10
	if v := os.Getenv("SSH_MENU_PAGE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pageSize = n
		}
	}

	listenAddr := ":2222"
	if port := os.Getenv("SSH_LISTEN_PORT"); port != "" {
		listenAddr = ":" + port
	}

	return SSHConfig{
		ListenAddr:        listenAddr,
		HostKeyPath:       os.Getenv("SSH_HOST_KEY_PATH"),
		CFCAPublicKeyPath: os.Getenv("CF_CA_PUBLIC_KEY_PATH"),
		QRouterNamespace:  os.Getenv("QROUTER_NAMESPACE"),
		ServiceKeyPath:    os.Getenv("RCP_SERVICE_KEY_PATH"),
		MenuPageSize:      pageSize,
	}
}

// UserVM represents a user's VM entry in the user_vms table.
type UserVM struct {
	ID        int64
	UserEmail string
	VMName    string
	VMID      string // OpenStack server UUID
	FixedIP   string // private IP in OpenStack network
	CreatedAt time.Time
}

// SSHSession represents an in-memory active SSH session (not persisted to DB).
type SSHSession struct {
	ID          string
	UserEmail   string
	VMID        string
	VMName      string
	VMip        string
	ConnectedAt time.Time
}
