package compute

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/gophercloud/gophercloud"
)

func Init(p *gophercloud.ProviderClient, projectID string, client *ent.Client) *Handler {
	osClient := NewClient(p)
	repo := NewRepository(client)
	svc := NewService(osClient, projectID, repo)
	return NewHandler(svc)
}
