package access

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
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
		Region: "RegionOne",
	})
}

func (c *Client) GetKeyPair(name string) (*KeyPair, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	kp, err := keypairs.Get(sc, name, nil).Extract()
	if err != nil {
		return nil, toStatusError(err)
	}

	return &KeyPair{
		Name:        kp.Name,
		Fingerprint: kp.Fingerprint,
		PublicKey:   kp.PublicKey,
	}, nil
}

func (c *Client) ListKeyPairs() ([]KeyPair, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	pages, err := keypairs.List(sc, nil).AllPages()
	if err != nil {
		return nil, toStatusError(err)
	}

	kps, err := keypairs.ExtractKeyPairs(pages)
	if err != nil {
		return nil, err
	}

	result := make([]KeyPair, len(kps))
	for i, kp := range kps {
		result[i] = KeyPair{
			Name:        kp.Name,
			Fingerprint: kp.Fingerprint,
			PublicKey:   kp.PublicKey,
		}
	}
	return result, nil
}

func (c *Client) DeleteKeyPair(name string) error {
	sc, err := c.serviceClient()
	if err != nil {
		return err
	}

	return toStatusError(keypairs.Delete(sc, name, nil).ExtractErr())
}

func (c *Client) CreateKeyPair(name, publicKey string) (*KeyPair, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	kp, err := keypairs.Create(sc, keypairs.CreateOpts{
		Name:      name,
		PublicKey: publicKey,
	}).Extract()
	if err != nil {
		return nil, toStatusError(err)
	}

	return &KeyPair{
		Name:        kp.Name,
		Fingerprint: kp.Fingerprint,
		PublicKey:   kp.PublicKey,
	}, nil
}

func (c *Client) GetInstance(id string) (*ConsoleInstance, error) {
	sc, err := c.serviceClient()
	if err != nil {
		return nil, err
	}

	server, err := servers.Get(sc, id).Extract()
	if err != nil {
		return nil, toStatusError(err)
	}

	fixedIP, floatingIP := extractServerIPs(server.Addresses, server.AccessIPv4)
	return &ConsoleInstance{
		ID:         server.ID,
		Name:       server.Name,
		FixedIP:    fixedIP,
		FloatingIP: floatingIP,
	}, nil
}

func toStatusError(err error) error {
	if err == nil {
		return nil
	}

	codes := []int{
		http.StatusBadRequest,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusInternalServerError,
	}

	for _, code := range codes {
		if isGophercloudStatus(err, code) {
			return &StatusError{Code: code, Err: err}
		}
	}

	return err
}

func isGophercloudStatus(err error, code int) bool {
	switch code {
	case http.StatusBadRequest:
		var e gophercloud.ErrDefault400
		if errors.As(err, &e) {
			return true
		}
	case http.StatusForbidden:
		var e gophercloud.ErrDefault403
		if errors.As(err, &e) {
			return true
		}
	case http.StatusNotFound:
		var e gophercloud.ErrDefault404
		if errors.As(err, &e) {
			return true
		}
	case http.StatusConflict:
		var e gophercloud.ErrDefault409
		if errors.As(err, &e) {
			return true
		}
	}

	var codeErr gophercloud.StatusCodeError
	return errors.As(err, &codeErr) && codeErr.GetStatusCode() == code
}

type serverAddress struct {
	Address string `json:"addr"`
	Type    string `json:"OS-EXT-IPS:type"`
}

func extractServerIPs(addresses map[string]interface{}, accessIPv4 string) (string, string) {
	var fixedIP string
	var floatingIP string

	for _, rawAddresses := range addresses {
		for _, address := range decodeServerAddresses(rawAddresses) {
			ip := strings.TrimSpace(address.Address)
			if ip == "" {
				continue
			}

			switch strings.TrimSpace(address.Type) {
			case "fixed":
				if fixedIP == "" {
					fixedIP = ip
				}
			case "floating":
				if floatingIP == "" {
					floatingIP = ip
				}
			}
		}
	}

	if floatingIP == "" {
		floatingIP = strings.TrimSpace(accessIPv4)
	}

	return fixedIP, floatingIP
}

func decodeServerAddresses(rawAddresses interface{}) []serverAddress {
	if rawAddresses == nil {
		return nil
	}

	b, err := json.Marshal(rawAddresses)
	if err != nil {
		return nil
	}

	var decoded []serverAddress
	if err := json.Unmarshal(b, &decoded); err != nil {
		return nil
	}
	return decoded
}
