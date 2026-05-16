package storage

import (
	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/ent"
)

func Init(provider *gophercloud.ProviderClient, entClient *ent.Client) *Handler {
	client := NewClient(provider)
	repo := NewRepository(entClient)
	svc := NewService(client, repo)
	return NewHandler(svc)
}
