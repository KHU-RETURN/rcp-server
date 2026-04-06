package access

import "github.com/gophercloud/gophercloud"

func Init(p *gophercloud.ProviderClient) *Handler {
	client := NewClient(p)
	svc := NewService(client)
	return NewHandler(svc)
}
