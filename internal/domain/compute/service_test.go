package compute

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type fakeRepository struct {
	fetchFlavorsFn     func() ([]flavors.Flavor, error)
	getComputeQuotaFn  func(client *gophercloud.ServiceClient, projectID string) (*quotasets.QuotaDetailSet, error)
	getComputeClientFn func() (*gophercloud.ServiceClient, error)
	createServerFn     func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error)
}

func (f *fakeRepository) FetchFlavors() ([]flavors.Flavor, error) {
	if f.fetchFlavorsFn != nil {
		return f.fetchFlavorsFn()
	}
	return nil, nil
}

func (f *fakeRepository) GetComputeQuota(client *gophercloud.ServiceClient, projectID string) (*quotasets.QuotaDetailSet, error) {
	if f.getComputeQuotaFn != nil {
		return f.getComputeQuotaFn(client, projectID)
	}
	return nil, nil
}

func (f *fakeRepository) GetComputeClient() (*gophercloud.ServiceClient, error) {
	if f.getComputeClientFn != nil {
		return f.getComputeClientFn()
	}
	return nil, nil
}

func (f *fakeRepository) CreateServer(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
	if f.createServerFn != nil {
		return f.createServerFn(client, opts)
	}
	return nil, nil
}

func TestServiceCreateInstance(t *testing.T) {
	t.Run("maps floating and fixed IPs into response", func(t *testing.T) {
		var gotOpts CreateServerOpts
		repo := &fakeRepository{
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				gotOpts = opts
				return testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.8", "OS-EXT-IPS:type": "fixed"},
						map[string]any{"addr": "203.0.113.10", "OS-EXT-IPS:type": "floating"},
					},
				}), nil
			},
		}

		svc := NewService(repo)
		res, err := svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:           " test-vm ",
			ImageRef:       " image-1 ",
			FlavorRef:      " flavor-1 ",
			KeyName:        " team-key ",
			SecurityGroups: []string{" default ", " ", "ssh"},
			Networks:       []servers.Network{{UUID: " network-1 "}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if gotOpts.Name != "test-vm" || gotOpts.ImageRef != "image-1" || gotOpts.FlavorRef != "flavor-1" {
			t.Fatalf("expected normalized create opts, got %+v", gotOpts)
		}
		if gotOpts.KeyName != "team-key" {
			t.Fatalf("expected trimmed key name, got %q", gotOpts.KeyName)
		}
		if !reflect.DeepEqual(gotOpts.SecurityGroups, []string{"default", "ssh"}) {
			t.Fatalf("expected normalized security groups, got %#v", gotOpts.SecurityGroups)
		}
		if len(gotOpts.Networks) != 1 || gotOpts.Networks[0].UUID != "network-1" {
			t.Fatalf("expected trimmed network UUID, got %#v", gotOpts.Networks)
		}

		if res.FixedIP != "10.0.0.8" {
			t.Fatalf("expected fixed_ip 10.0.0.8, got %q", res.FixedIP)
		}
		if res.FloatingIP != "203.0.113.10" {
			t.Fatalf("expected floating_ip 203.0.113.10, got %q", res.FloatingIP)
		}
		if !reflect.DeepEqual(res.SecurityGroups, []string{"default", "ssh"}) {
			t.Fatalf("expected security groups to be mapped, got %#v", res.SecurityGroups)
		}
	})

	t.Run("falls back to fixed IP when floating IP is missing", func(t *testing.T) {
		repo := &fakeRepository{
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				return testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.9", "OS-EXT-IPS:type": "fixed"},
					},
				}), nil
			},
		}

		svc := NewService(repo)
		res, err := svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:      "test-vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
			KeyName:   "team-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.FloatingIP != "" {
			t.Fatalf("expected empty floating_ip, got %q", res.FloatingIP)
		}
		if res.FixedIP != "10.0.0.9" {
			t.Fatalf("expected fixed_ip 10.0.0.9, got %q", res.FixedIP)
		}
	})

	t.Run("uses access ipv4 as floating IP fallback", func(t *testing.T) {
		repo := &fakeRepository{
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				server := testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.10", "OS-EXT-IPS:type": "fixed"},
					},
				})
				server.AccessIPv4 = "198.51.100.24"
				return server, nil
			},
		}

		svc := NewService(repo)
		res, err := svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:      "test-vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
			KeyName:   "team-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.FloatingIP != "198.51.100.24" {
			t.Fatalf("expected floating_ip to use accessIPv4, got %q", res.FloatingIP)
		}
	})

	t.Run("falls back to request values when server metadata is missing", func(t *testing.T) {
		repo := &fakeRepository{
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				return &servers.Server{
					ID:        "server-2",
					Name:      "fallback-vm",
					Status:    "BUILD",
					Addresses: map[string]any{},
				}, nil
			},
		}

		svc := NewService(repo)
		res, err := svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:           "fallback-vm",
			ImageRef:       "image-2",
			FlavorRef:      "flavor-2",
			KeyName:        "team-key",
			SecurityGroups: []string{"default", "ssh"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.ImageID != "image-2" || res.FlavorID != "flavor-2" {
			t.Fatalf("expected request image/flavor fallback, got image=%q flavor=%q", res.ImageID, res.FlavorID)
		}
		if res.KeyName != "team-key" {
			t.Fatalf("expected request key_name fallback, got %q", res.KeyName)
		}
		if !reflect.DeepEqual(res.SecurityGroups, []string{"default", "ssh"}) {
			t.Fatalf("expected request security_groups fallback, got %#v", res.SecurityGroups)
		}
	})

	t.Run("rejects whitespace-only required fields", func(t *testing.T) {
		svc := NewService(&fakeRepository{})

		_, err := svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:      "   ",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
		})
		if err != ErrCreateInstanceNameRequired {
			t.Fatalf("expected ErrCreateInstanceNameRequired, got %v", err)
		}

		_, err = svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "   ",
			FlavorRef: "flavor-1",
		})
		if err != ErrCreateInstanceImageRequired {
			t.Fatalf("expected ErrCreateInstanceImageRequired, got %v", err)
		}

		_, err = svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "image-1",
			FlavorRef: "   ",
		})
		if err != ErrCreateInstanceFlavorRequired {
			t.Fatalf("expected ErrCreateInstanceFlavorRequired, got %v", err)
		}
	})

	t.Run("drops blank network entries after trimming", func(t *testing.T) {
		var gotOpts CreateServerOpts
		repo := &fakeRepository{
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				gotOpts = opts
				return testServer(map[string]any{}), nil
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateInstance(&gophercloud.ServiceClient{}, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
			Networks:  []servers.Network{{UUID: "   "}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gotOpts.Networks) != 0 {
			t.Fatalf("expected blank networks to be dropped, got %#v", gotOpts.Networks)
		}
	})
}

func testServer(addresses map[string]any) *servers.Server {
	return &servers.Server{
		ID:     "server-1",
		Name:   "test-vm",
		Status: "BUILD",
		Image: map[string]any{
			"id": "image-1",
		},
		Flavor: map[string]any{
			"id": "flavor-1",
		},
		Addresses: addresses,
		KeyName:   "team-key",
		SecurityGroups: []map[string]any{
			{"name": "default"},
			{"name": "ssh"},
		},
	}
}
