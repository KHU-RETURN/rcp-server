package server

import (
	"database/sql"
	"fmt"
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
	authHandler, err := auth.Init(db, oauthConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth module: %w", err)
	}

	return &App{
		Compute: compute.Init(p),
		Access:  access.Init(p),
		Auth:    authHandler,
	}, nil
}
