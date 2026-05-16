package access

import (
	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/ent"
)

func Init(provider *gophercloud.ProviderClient, entClient *ent.Client) *Handler {
	osClient := NewClient(provider)
	repo := NewRepository(entClient)
	svc := NewService(osClient, repo)
	return NewHandler(svc)
}
