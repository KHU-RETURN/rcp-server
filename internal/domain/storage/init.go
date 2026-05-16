package storage

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/gophercloud/gophercloud"
)

func Init(provider *gophercloud.ProviderClient, entClient *ent.Client) *Handler {
	client := NewClient(provider)
	repo := NewRepository(entClient)
	svc := NewService(client, repo)
	return NewHandler(svc)
}
