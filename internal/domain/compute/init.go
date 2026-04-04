package compute

import "github.com/gophercloud/gophercloud"

func Init(p *gophercloud.ProviderClient, projectID string) *Handler {
	client := NewClient(p)
	svc := NewService(client, projectID)
	return NewHandler(svc)
}
