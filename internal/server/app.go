package server

import (
	"fmt"

	"github.com/KHU-RETURN/rcp-server/ent"
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
	client *ent.Client,
	oauthConfig *oauth2.Config,
	projectID string,
	jwtSecret string,
	defaultNetworkID string,
) (*App, error) {
	authHandler, err := auth.Init(client, oauthConfig, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth: %w", err)
	}

	return &App{
		Compute: compute.Init(p, client, projectID, defaultNetworkID),
		Access:  access.Init(p, client),
		Auth:    authHandler,
	}, nil
}
