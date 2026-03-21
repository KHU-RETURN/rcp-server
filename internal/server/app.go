package server

import (
	"database/sql"

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
) *App {
	return &App{
		Compute: compute.Init(p),
		Access:  access.Init(p),
		Auth:    auth.Init(db, oauthConfig),
	}
}
