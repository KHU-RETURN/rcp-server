package server

import (
	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/gophercloud/gophercloud"
)

type App struct {
	Access  *access.Handler
	Compute *compute.Handler
}

func NewApp(p *gophercloud.ProviderClient) *App {
	return &App{
		Access:  access.Init(p),
		Compute: compute.Init(p),
	}
}
