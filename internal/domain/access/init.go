package access

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/gophercloud/gophercloud"
)

func Init(p *gophercloud.ProviderClient, client *ent.Client) *Handler {
	osClient := NewClient(p)
	repo := NewRepository(client)
	svc := NewService(osClient, repo)
	return NewHandler(svc)
}
