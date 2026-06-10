package admin

import (
	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/ent"
)

type Option func(*options)

type options struct {
	health       healthChecker
	statusSource instanceStatusSource
}

func WithHealthChecker(health healthChecker) Option {
	return func(opts *options) {
		opts.health = health
	}
}

func WithLiveHealthChecker(provider *gophercloud.ProviderClient, sshGatewaySock, nsProxySock, httpProxyAddress string) Option {
	return WithHealthChecker(NewLiveHealthChecker(provider, sshGatewaySock, nsProxySock, httpProxyAddress))
}

func WithInstanceStatusSource(source instanceStatusSource) Option {
	return func(opts *options) {
		opts.statusSource = source
	}
}

func WithLiveInstanceStatusSource(provider *gophercloud.ProviderClient) Option {
	return WithInstanceStatusSource(NewLiveInstanceStatusSource(provider))
}

func Init(entClient *ent.Client, opts ...Option) *Handler {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}
	repo := NewRepository(entClient)
	svc := NewService(repo, cfg.health, cfg.statusSource)
	return NewHandler(svc)
}
