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
	repo, err := NewSSHRepository(db)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: repository: %w", err)
	}

	hostKey, err := loadSSHHostKey(cfg.HostKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: host key: %w", err)
	}

	cfCAKey, err := loadCFCAKey(cfg.CFCAPublicKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: CF CA key: %w", err)
	}

	serviceKey, err := loadSSHServiceKey(cfg.ServiceKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh init: service key: %w", err)
	}

	serverConfig := buildSSHServerConfig(hostKey, cfCAKey, verifier)
	dialer := &NamespaceDialer{Namespace: cfg.QRouterNamespace}
	svc := NewSSHService(repo, dialer, serviceKey, cfg.MenuPageSize)
	handler := NewConnectionHandler(svc)
	server := NewSSHServer(cfg, serverConfig, handler)

	return server, repo, nil
}
