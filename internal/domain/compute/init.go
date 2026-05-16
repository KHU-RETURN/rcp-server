package compute

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/gophercloud/gophercloud"
)

func Init(p *gophercloud.ProviderClient, entClient *ent.Client, projectID, defaultNetworkID string) *Handler {
	client := NewClient(p)
	repo := NewRepository(entClient)
	svc := NewService(client, repo, projectID, defaultNetworkID)
	return NewHandler(svc)
}
