package admin

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"

	rcpopenstack "github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
)

var ErrHealthCheckUnconfigured = errors.New("health check unconfigured")

type healthChecker interface {
	CheckOpenStack(ctx context.Context) error
	CheckStorage(ctx context.Context) error
	CheckSSHGateway(ctx context.Context) error
}

type liveHealthChecker struct {
	provider       *gophercloud.ProviderClient
	sshGatewaySock string
	timeout        time.Duration
}

func NewLiveHealthChecker(provider *gophercloud.ProviderClient, sshGatewaySock string) healthChecker {
	return &liveHealthChecker{
		provider:       provider,
		sshGatewaySock: sshGatewaySock,
		timeout:        2 * time.Second,
	}
}

func (c *liveHealthChecker) CheckOpenStack(ctx context.Context) error {
	if c == nil || c.provider == nil {
		return ErrHealthCheckUnconfigured
	}
	provider, cancel := c.providerWithContext(ctx)
	defer cancel()

	sc, err := goopenstack.NewComputeV2(provider, gophercloud.EndpointOpts{
		Region: rcpopenstack.Region,
	})
	if err != nil {
		return err
	}
	resp, err := sc.Get(sc.Endpoint, nil, &gophercloud.RequestOpts{OkCodes: []int{200, 203, 204, 300}})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return err
}

func (c *liveHealthChecker) CheckStorage(ctx context.Context) error {
	if c == nil || c.provider == nil {
		return ErrHealthCheckUnconfigured
	}
	provider, cancel := c.providerWithContext(ctx)
	defer cancel()

	sc, err := goopenstack.NewObjectStorageV1(provider, gophercloud.EndpointOpts{
		Region: rcpopenstack.Region,
	})
	if err != nil {
		return err
	}
	resp, err := sc.Head(sc.Endpoint, &gophercloud.RequestOpts{OkCodes: []int{200, 203, 204, 300}})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return err
}

func (c *liveHealthChecker) CheckSSHGateway(ctx context.Context) error {
	if c == nil || c.sshGatewaySock == "" {
		return ErrHealthCheckUnconfigured
	}
	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "unix", c.sshGatewaySock)
	if err != nil {
		return err
	}
	return conn.Close()
}

func (c *liveHealthChecker) providerWithContext(parent context.Context) (*gophercloud.ProviderClient, context.CancelFunc) {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	provider := *c.provider
	provider.Context = ctx
	return &provider, cancel
}

func healthStatus(err error) string {
	if err == nil {
		return "healthy"
	}
	if errors.Is(err, ErrHealthCheckUnconfigured) {
		return "unconfigured"
	}
	return "unhealthy"
}
