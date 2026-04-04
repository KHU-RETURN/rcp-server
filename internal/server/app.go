package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/gophercloud/gophercloud"
	"golang.org/x/oauth2"
)

type App struct {
	Compute *compute.Handler
	Access  *access.Handler
	Auth    *auth.Handler
	SSH     *access.SSHServer
}

func NewApp(
	p *gophercloud.ProviderClient,
	db *sql.DB,
	oauthConfig *oauth2.Config,
) (*App, error) {
	// Auth repository is constructed separately so it can be shared with the SSH domain
	authRepo, err := auth.NewRepository(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth repository: %w", err)
	}
	authHandler, err := buildAuthHandler(authRepo, oauthConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth module: %w", err)
	}

	// SSH relay server (access domain)
	sshCfg := access.SSHConfigFromEnv()
	verifier := &userVerifierAdapter{repo: authRepo}
	sshServer, vmRepo, err := access.InitSSH(db, verifier, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SSH relay: %w", err)
	}

	return &App{
		Compute: compute.Init(p, vmRepo),
		Access:  access.Init(p),
		Auth:    authHandler,
		SSH:     sshServer,
	}, nil
}

// buildAuthHandler replicates auth.Init logic using an already-constructed repository.
func buildAuthHandler(repo auth.UserRepository, oauthConfig *oauth2.Config) (*auth.Handler, error) {
	secret := os.Getenv("RCP_JWT_SECRET")
	if secret == "" {
		secret = "default-low-security-key-for-dev" // #nosec G101
	}
	tokenSvc := auth.NewTokenService(secret)
	svc := auth.NewService(repo, oauthConfig, tokenSvc)
	return auth.NewHandler(svc), nil
}

// userVerifierAdapter adapts auth.UserRepository to access.UserVerifier.
type userVerifierAdapter struct {
	repo auth.UserRepository
}

func (a *userVerifierAdapter) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	user, err := a.repo.FindByEmail(ctx, email)
	if err != nil {
		return false, err
	}
	return user != nil, nil
}
