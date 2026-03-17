package server

import (
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/gophercloud/gophercloud"
)

type App struct {
	Compute *compute.Handler
}

func NewApp(p *gophercloud.ProviderClient) *App {
	return &App{
		Compute: compute.Init(p),
	}
}
