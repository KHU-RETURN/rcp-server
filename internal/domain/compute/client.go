package compute

import (
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/diagnostics"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type Client struct {
	provider *gophercloud.ProviderClient
}

func NewClient(provider *gophercloud.ProviderClient) *Client {
	return &Client{provider: provider}
}

func (c *Client) serviceClient() (*gophercloud.ServiceClient, error) {
	return goopenstack.NewComputeV2(c.provider, gophercloud.EndpointOpts{
		Region: openstack.Region,
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

func (c *Client) FetchInstances() ([]Server, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	allPages, err := servers.List(sc, servers.ListOpts{}).AllPages()
	if err != nil {
		return nil, err
	}

	raw, err := servers.ExtractServers(allPages)
	if err != nil {
		return nil, err
	}

	result := make([]Server, len(raw))
	for i, s := range raw {
		result[i] = Server{
			ID:         s.ID,
			Name:       s.Name,
			Status:     s.Status,
			Addresses:  s.Addresses,
			AccessIPv4: s.AccessIPv4,
		}
	}
	return result, nil
}

func (c *Client) FetchInstance(id string) (*Server, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	raw, err := servers.Get(sc, id).Extract()
	if err != nil {
		return nil, err
	}

	return &Server{
		ID:         raw.ID,
		Name:       raw.Name,
		Status:     raw.Status,
		Addresses:  raw.Addresses,
		AccessIPv4: raw.AccessIPv4,
	}, nil
}

func (c *Client) FetchDiagnostics(id string) (map[string]any, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}
	return diagnostics.Get(sc, id).Extract()
}

func (c *Client) DeleteServer(id string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}
	return servers.Delete(sc, id).ExtractErr()
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
