package access

import (
	"database/sql"
	"fmt"

	"github.com/gophercloud/gophercloud"
)

// Init wires the keypair HTTP handler.
func Init(p *gophercloud.ProviderClient) *Handler {
	repo := NewRepository(p)
	svc := NewService(repo)
	return NewHandler(svc)
}

// InitSSH wires the SSH relay server.
//
// Returns the SSHServer (call ListenAndServe in a goroutine) and the
// SSHRepository (pass to compute.Init as the VMRegistrar).
func InitSSH(db *sql.DB, verifier UserVerifier, cfg SSHConfig) (*SSHServer, *SSHRepository, error) {
	// 1. Repository: creates user_vms table
	repo, err := NewSSHRepository(db)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: repository: %w", err)
	}

	// 2. Load SSH host key
	hostKey, err := loadSSHHostKey(cfg.HostKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: host key: %w", err)
	}

	// 3. Load Cloudflare CA public key for cert verification
	cfCAKey, err := loadCFCAKey(cfg.CFCAPublicKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: CF CA key: %w", err)
	}

	// 4. Build gossh.ServerConfig with CF cert checker
	serverConfig := buildSSHServerConfig(hostKey, cfCAKey, verifier)

	// 5. Load RCP service key (used to authenticate RCP → VM)
	serviceKey, err := loadSSHServiceKey(cfg.ServiceKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: service key: %w", err)
	}

	// 6. Namespace dialer
	dialer := &NamespaceDialer{Namespace: cfg.QRouterNamespace}

	// 7. SSH service
	svc := NewSSHService(repo, dialer, serviceKey, cfg.MenuPageSize)

	// 8. Connection handler
	handler := NewConnectionHandler(svc)

	// 9. SSH server
	server := NewSSHServer(cfg, serverConfig, handler)

	return server, repo, nil
}
