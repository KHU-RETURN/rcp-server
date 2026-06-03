package server

import (
	"fmt"

	"github.com/gophercloud/gophercloud"
	"golang.org/x/oauth2"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/apps"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/KHU-RETURN/rcp-server/internal/domain/storage"
)

type App struct {
	Compute *compute.Handler
	Access  *access.Handler
	Apps    *apps.Handler
	Auth    *auth.Handler
	Storage *storage.Handler
}

type AppDeps struct {
	Provider         *gophercloud.ProviderClient
	EntClient        *ent.Client
	OAuthConfig      *oauth2.Config
	OpenStackProject string
	DefaultNetworkID string
	JWTSecret        string
	SSHGatewaySock   string
	SSHGatewaySecret []byte
	FrontendBaseURL  string
	UsageLimits      compute.UserUsageLimits
	StorageLimits    storage.UserStorageLimits
}

func NewApp(deps AppDeps) (*App, error) {
	var sshSvc *access.SSHService
	if deps.SSHGatewaySock != "" && len(deps.SSHGatewaySecret) > 0 {
		sshSvc = access.InitSSH(deps.SSHGatewaySock, deps.SSHGatewaySecret)
	}

	authHandler, err := auth.Init(deps.EntClient, deps.OAuthConfig, deps.JWTSecret, sshSvc, deps.FrontendBaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth: %w", err)
	}
	return &App{
		Compute: compute.Init(deps.Provider, deps.EntClient, deps.OpenStackProject, deps.DefaultNetworkID, deps.UsageLimits),
		Access:  access.Init(deps.Provider, deps.EntClient, deps.SSHGatewaySecret),
		Apps:    apps.Init(deps.EntClient),
		Auth:    authHandler,
		Storage: storage.Init(deps.Provider, deps.EntClient, deps.StorageLimits),
	}, nil
}
