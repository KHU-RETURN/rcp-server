package admin

import (
	"context"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
)

type instanceStatusSource interface {
	InstanceStatuses(ctx context.Context) (map[string]string, error)
	InstanceStatus(ctx context.Context, id string) (string, error)
}

// computeStatusClient is the subset of the compute client used to read live statuses.
type computeStatusClient interface {
	FetchInstances() ([]compute.Server, error)
	FetchInstance(id string) (*compute.Server, error)
}

// statusCacheTTL — every admin list request would otherwise list all servers
// in the project, so keep the ID→status map briefly cached.
const statusCacheTTL = 15 * time.Second

// liveInstanceStatusSource reuses the compute client so the admin views read the
// same live OpenStack status as the user-facing compute views.
type liveInstanceStatusSource struct {
	client computeStatusClient

	mu     sync.Mutex
	cache  map[string]string
	expire time.Time
}

func NewLiveInstanceStatusSource(provider *gophercloud.ProviderClient) instanceStatusSource {
	return &liveInstanceStatusSource{client: compute.NewClient(provider)}
}

func (s *liveInstanceStatusSource) InstanceStatuses(context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.expire) {
		return s.cache, nil
	}
	servers, err := s.client.FetchInstances()
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]string, len(servers))
	for _, srv := range servers {
		statuses[srv.ID] = srv.Status
	}
	s.cache = statuses
	s.expire = time.Now().Add(statusCacheTTL)
	return statuses, nil
}

func (s *liveInstanceStatusSource) InstanceStatus(_ context.Context, id string) (string, error) {
	srv, err := s.client.FetchInstance(id)
	if err != nil {
		return "", err
	}
	return srv.Status, nil
}
