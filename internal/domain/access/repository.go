package access

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
)

type Repository struct {
	Client *gophercloud.ProviderClient
}

func NewRepository(client *gophercloud.ProviderClient) *Repository {
	return &Repository{Client: client}
}

func (r *Repository) GetComputeClient() (*gophercloud.ServiceClient, error) {
	return openstack.NewComputeV2(r.Client, gophercloud.EndpointOpts{
		Region: "RegionOne",
	})
}

func (r *Repository) GetKeyPair(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
	return keypairs.Get(client, name, nil).Extract()
}

func (r *Repository) CreateKeyPair(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
	return keypairs.Create(client, keypairs.CreateOpts{
		Name:      name,
		PublicKey: publicKey,
	}).Extract()
}
