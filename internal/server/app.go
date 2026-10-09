package server

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud"
	"golang.org/x/oauth2"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/admin"
	"github.com/KHU-RETURN/rcp-server/internal/domain/apps"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/KHU-RETURN/rcp-server/internal/domain/blockstorage"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/KHU-RETURN/rcp-server/internal/domain/functions"
	"github.com/KHU-RETURN/rcp-server/internal/domain/operations"
	"github.com/KHU-RETURN/rcp-server/internal/domain/storage"
)

type App struct {
	BlockStorage *blockstorage.Handler
	Compute      *compute.Handler
	Access       *access.Handler
	Admin        *admin.Handler
	Apps         *apps.Handler
	Auth         *auth.Handler
	Storage      *storage.Handler
	Functions    *functions.Handler
	Operations   *operations.Handler
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
	NSProxySock      string
	HTTPProxyAddress string
	FrontendBaseURL  string
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
	functionHandler, err := functions.Init(context.Background(), deps.EntClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize functions: %w", err)
	}
	backend := operations.NewLiveBackend(deps.EntClient, deps.Provider, deps.DefaultNetworkID)
	queue := operations.NewService(deps.EntClient, backend)
	app := &App{
		BlockStorage: blockstorage.Init(deps.Provider, deps.EntClient),
		Compute:      compute.Init(deps.Provider, deps.EntClient, deps.OpenStackProject, deps.DefaultNetworkID),
		Access:       access.Init(deps.Provider, deps.EntClient, deps.SSHGatewaySecret),
		Admin: admin.Init(
			deps.EntClient,
			admin.WithLiveHealthChecker(deps.Provider, deps.SSHGatewaySock, deps.NSProxySock, deps.HTTPProxyAddress),
			admin.WithLiveInstanceStatusSource(deps.Provider),
		),
		Apps:       apps.Init(deps.EntClient),
		Auth:       authHandler,
		Storage:    storage.Init(deps.Provider, deps.EntClient),
		Functions:  functionHandler,
		Operations: &operations.Handler{Svc: queue},
	}
	app.Compute.Queue = queue
	app.Storage.Queue = queue
	app.BlockStorage.Queue = queue
	return app, nil
}

func (a *App) Close(ctx context.Context) error {
	if a.Operations != nil {
		if err := a.Operations.Svc.Close(ctx); err != nil {
			return err
		}
	}
	if a.Functions != nil {
		return a.Functions.Svc.Close(ctx)
	}
	return nil
}
