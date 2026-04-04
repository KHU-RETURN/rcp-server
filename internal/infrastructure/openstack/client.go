package openstack

import (
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/http"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
)

type ProviderConfig struct {
	AuthURL    string
	Username   string
	Password   string
	ProjectName string
	DomainName string
	CFClientID string
	CFSecret   string
}

func NewProviderClient(cfg ProviderConfig) (*gophercloud.ProviderClient, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: cfg.AuthURL,
		Username:         cfg.Username,
		Password:         cfg.Password,
		TenantName:       cfg.ProjectName,
		DomainName:       cfg.DomainName,
	}

	provider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, err
	}

	provider.HTTPClient = *http.NewCloudflareClient(cfg.CFClientID, cfg.CFSecret)

	err = openstack.Authenticate(provider, opts)
	return provider, err
}
