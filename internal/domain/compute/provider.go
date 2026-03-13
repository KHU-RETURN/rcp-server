package compute

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
)

type Provider struct {
	Client *gophercloud.ProviderClient
}

type AuthConfig struct {
	IdentityEndpoint string // 인증 URL (예: http://openstack-ip:5000/v3)
	Username         string
	Password         string
	DomainName       string
	ProjectName      string
}

func NewProvider(cfg AuthConfig) (*Provider, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: cfg.IdentityEndpoint,
		Username:         cfg.Username,
		Password:         cfg.Password,
		DomainName:       cfg.DomainName,
		TenantName:       cfg.ProjectName,
	}

	// 실제 오픈스택 서버와 인증 수행
	client, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		return nil, err
	}

	return &Provider{Client: client}, nil
}
