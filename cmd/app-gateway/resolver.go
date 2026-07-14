package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	gccompute "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type fixedIPResolver interface {
	ResolveFixedIPv4(ctx context.Context, openstackID string) (string, error)
}

type gopherResolver struct {
	compute      *gophercloud.ServiceClient
	fixedNetwork string
}

func newGopherResolver(p *gophercloud.ProviderClient, fixedNetwork string) (*gopherResolver, error) {
	// Without a timeout a hung OpenStack API blocks forever; the caller's
	// context is also honored per call via computeWithContext.
	if p.HTTPClient.Timeout <= 0 {
		p.HTTPClient.Timeout = 15 * time.Second
	}
	c, err := goopenstack.NewComputeV2(p, gophercloud.EndpointOpts{Region: openstack.Region})
	if err != nil {
		return nil, err
	}
	return &gopherResolver{compute: c, fixedNetwork: strings.TrimSpace(fixedNetwork)}, nil
}

func (g *gopherResolver) ResolveFixedIPv4(ctx context.Context, openstackID string) (string, error) {
	srv, err := gccompute.Get(g.computeWithContext(ctx), openstackID).Extract()
	if err != nil {
		return "", err
	}
	return fixedIPv4FromAddresses(srv.Addresses, g.fixedNetwork)
}

// computeWithContext returns a shallow copy of the compute client whose
// provider carries ctx, so gophercloud requests respect caller deadlines.
func (g *gopherResolver) computeWithContext(ctx context.Context) *gophercloud.ServiceClient {
	provider := *g.compute.ProviderClient
	provider.Context = ctx
	client := *g.compute
	client.ProviderClient = &provider
	return &client
}

func fixedIPv4FromAddresses(addresses map[string]any, fixedNetwork string) (string, error) {
	fixedNetwork = strings.TrimSpace(fixedNetwork)
	var fixedIPs []string
	for network, raw := range addresses {
		if fixedNetwork != "" && network != fixedNetwork {
			continue
		}
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			m, ok := e.(map[string]any)
			if !ok || !isIPv4Version(m["version"]) {
				continue
			}
			if ipType, _ := m["OS-EXT-IPS:type"].(string); strings.TrimSpace(ipType) != "fixed" {
				continue
			}
			if addr, _ := m["addr"].(string); strings.TrimSpace(addr) != "" {
				fixedIPs = append(fixedIPs, strings.TrimSpace(addr))
			}
		}
	}
	switch len(fixedIPs) {
	case 0:
		if fixedNetwork != "" {
			return "", fmt.Errorf("no fixed IPv4 address on network %q", fixedNetwork)
		}
		return "", errors.New("no fixed IPv4 address on server")
	case 1:
		return fixedIPs[0], nil
	default:
		return "", fmt.Errorf("multiple fixed IPv4 addresses found; set RCP_APP_GW_FIXED_NETWORK")
	}
}

func isIPv4Version(v any) bool {
	switch n := v.(type) {
	case float64:
		return n == 4
	case int:
		return n == 4
	case string:
		return n == "4"
	default:
		return false
	}
}
