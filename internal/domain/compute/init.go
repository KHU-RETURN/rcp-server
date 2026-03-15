package compute

import "github.com/gophercloud/gophercloud"

func Init(p *gophercloud.ProviderClient) *Handler {
	repo := NewRepository(p)
	svc := NewService(repo)
	return NewHandler(svc)
}
