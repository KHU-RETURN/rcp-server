package openstack

import (
	"os"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/http"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
)

func NewProviderClient() (*gophercloud.ProviderClient, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
		Username:         os.Getenv("OS_USERNAME"),
		Password:         os.Getenv("OS_PASSWORD"),
		TenantName:       os.Getenv("OS_PROJECT_NAME"),
		DomainName:       os.Getenv("OS_USER_DOMAIN_NAME"),
	}

	provider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, err
	}

	// Cloudflare 클라이언트 주입
	provider.HTTPClient = *http.NewCloudflareClient(
		os.Getenv("CF_ACCESS_CLIENT_ID"),
		os.Getenv("CF_ACCESS_CLIENT_SECRET"),
	)

	err = openstack.Authenticate(provider, opts)
	return provider, err
}