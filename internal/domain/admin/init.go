package admin

import (
	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/ent"
)

type Option func(*options)

type options struct {
	health healthChecker
}

func WithHealthChecker(health healthChecker) Option {
	return func(opts *options) {
		opts.health = health
	}
}

func WithLiveHealthChecker(provider *gophercloud.ProviderClient, sshGatewaySock string) Option {
	return WithHealthChecker(NewLiveHealthChecker(provider, sshGatewaySock))
}

func Init(entClient *ent.Client, opts ...Option) *Handler {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}
	repo := NewRepository(entClient)
	svc := NewService(repo, cfg.health)
	return NewHandler(svc)
}
