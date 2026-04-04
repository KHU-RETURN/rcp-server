package compute

import (
	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

// Client는 OpenStack compute API를 호출하는 구현체입니다.
type Client struct {
	provider *gophercloud.ProviderClient
}

func NewClient(provider *gophercloud.ProviderClient) *Client {
	return &Client{provider: provider}
}

func (c *Client) serviceClient() (*gophercloud.ServiceClient, error) {
	return goopenstack.NewComputeV2(c.provider, gophercloud.EndpointOpts{
		Region: "RegionOne",
	})
}

func (c *Client) FetchFlavors() ([]Flavor, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	allPages, err := flavors.ListDetail(sc, nil).AllPages()
	if err != nil {
		return nil, err
	}

	raw, err := flavors.ExtractFlavors(allPages)
	if err != nil {
		return nil, err
	}

	result := make([]Flavor, len(raw))
	for i, f := range raw {
		result[i] = Flavor{
			ID:    f.ID,
			Name:  f.Name,
			VCPUs: f.VCPUs,
			RAM:   f.RAM,
			Disk:  f.Disk,
		}
	}
	return result, nil
}

func (c *Client) GetComputeQuota(projectID string) (*QuotaDetailSet, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	detail, err := quotasets.GetDetail(sc, projectID).Extract()
	if err != nil {
		return nil, err
	}

	return &QuotaDetailSet{
		Cores:     QuotaDetail{InUse: detail.Cores.InUse, Limit: detail.Cores.Limit, Reserved: detail.Cores.Reserved},
		RAM:       QuotaDetail{InUse: detail.RAM.InUse, Limit: detail.RAM.Limit, Reserved: detail.RAM.Reserved},
		Instances: QuotaDetail{InUse: detail.Instances.InUse, Limit: detail.Instances.Limit, Reserved: detail.Instances.Reserved},
	}, nil
}

func (c *Client) CreateServer(opts CreateServerOpts) (*Server, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}
	return createServerWithServiceClient(sc, opts)
}

// createServerWithServiceClient는 테스트에서 mock ServiceClient를 주입할 때도 사용합니다.
func createServerWithServiceClient(sc *gophercloud.ServiceClient, opts CreateServerOpts) (*Server, error) {
	networks := make([]servers.Network, len(opts.Networks))
	for i, n := range opts.Networks {
		networks[i] = servers.Network{UUID: n.UUID}
	}

	baseOpts := servers.CreateOpts{
		Name:           opts.Name,
		ImageRef:       opts.ImageRef,
		FlavorRef:      opts.FlavorRef,
		SecurityGroups: opts.SecurityGroups,
		Networks:       networks,
	}

	createOpts := keypairs.CreateOptsExt{
		CreateOptsBuilder: baseOpts,
		KeyName:           opts.KeyName,
	}

	server, err := servers.Create(sc, createOpts).Extract()
	if err != nil {
		return nil, err
	}

	return &Server{
		ID:             server.ID,
		Name:           server.Name,
		Status:         server.Status,
		Image:          server.Image,
		Flavor:         server.Flavor,
		Addresses:      server.Addresses,
		KeyName:        server.KeyName,
		SecurityGroups: server.SecurityGroups,
		AccessIPv4:     server.AccessIPv4,
	}, nil
}
