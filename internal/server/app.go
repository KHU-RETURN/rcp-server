package server

import (
	"fmt"

	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	rcpssh "github.com/KHU-RETURN/rcp-server/internal/domain/ssh"
	"github.com/gophercloud/gophercloud"
	"golang.org/x/oauth2"
)

type App struct {
	Compute *compute.Handler
	Access  *access.Handler
	Auth    *auth.Handler
}

type AppDeps struct {
	Provider         *gophercloud.ProviderClient
	EntClient        *ent.Client
	OAuthConfig      *oauth2.Config
	OpenStackProject string
	JWTSecret        string
	SSHGatewaySock   string
	SSHGatewaySecret []byte
}

func NewApp(deps AppDeps) (*App, error) {
	var sshSvc *rcpssh.Service
	if deps.SSHGatewaySock != "" && len(deps.SSHGatewaySecret) > 0 {
		sshSvc = rcpssh.Init(deps.SSHGatewaySock, deps.SSHGatewaySecret)
	}

	authHandler, err := auth.Init(deps.EntClient, deps.OAuthConfig, deps.JWTSecret, sshSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth: %w", err)
	}
	return &App{
		Compute: compute.Init(deps.Provider, deps.EntClient, deps.OpenStackProject),
		Access:  access.Init(deps.Provider, deps.EntClient),
		Auth:    authHandler,
	}, nil
}
