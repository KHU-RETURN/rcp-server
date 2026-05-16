package compute

import (
	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/ent"
)

func Init(p *gophercloud.ProviderClient, entClient *ent.Client, projectID string) *Handler {
	client := NewClient(p)
	repo := NewRepository(entClient)
	svc := NewService(client, repo, projectID)
	return NewHandler(svc)
}
