package server

import (
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gophercloud/gophercloud"
    "golang.org/x/oauth2"
    "database/sql"
)

type App struct {
    Compute *compute.Handler
    Auth *auth.Handler
}

func NewApp(
	p *gophercloud.ProviderClient,
	db *sql.DB,
	oauthConfig *oauth2.Config,
) *App {
	return &App{
		Compute: compute.Init(p),
		Auth:    auth.Init(db, oauthConfig),
	}
}