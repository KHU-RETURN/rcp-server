package storage

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/gophercloud/gophercloud"
)

func Init(p *gophercloud.ProviderClient, entClient *ent.Client) *Handler {
	client := NewClient(p)
	repo := NewRepository(entClient)
	svc := NewService(client, repo)
	return NewHandler(svc)
}
