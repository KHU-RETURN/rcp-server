package access

import (
	"errors"
	"net/http"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
)

// Client는 OpenStack keypair API를 호출하는 구현체입니다.
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

// toStatusError는 gophercloud 에러를 *StatusError로 변환합니다.
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
