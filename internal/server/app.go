package server

import (
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
}

func NewApp(
	p *gophercloud.ProviderClient,
	db *sql.DB,
	oauthConfig *oauth2.Config,
) (*App, error) {
	// auth
	authRepo, err := auth.NewRepository(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth repository: %w", err)
	}
	secret := os.Getenv("RCP_JWT_SECRET")
	if secret == "" {
		secret = "default-low-security-key-for-dev" // #nosec G101
	}
	tokenSvc := auth.NewTokenService(secret)
	authSvc := auth.NewService(authRepo, oauthConfig, tokenSvc)
	authHandler := auth.NewHandler(authSvc)

	// compute
	computeClient := compute.NewClient(p)
	projectID := os.Getenv("OS_PROJECT_ID")
	computeSvc := compute.NewService(computeClient, projectID)
	computeHandler := compute.NewHandler(computeSvc)

	// access
	accessClient := access.NewClient(p)
	accessSvc := access.NewService(accessClient)
	accessHandler := access.NewHandler(accessSvc)

	return &App{
		Compute: computeHandler,
		Access:  accessHandler,
		Auth:    authHandler,
	}, nil
}
