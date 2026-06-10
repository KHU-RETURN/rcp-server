package admin

import (
	"context"

	"github.com/gophercloud/gophercloud"

	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
)

type instanceStatusSource interface {
	InstanceStatuses(ctx context.Context) (map[string]string, error)
	InstanceStatus(ctx context.Context, id string) (string, error)
}

// liveInstanceStatusSource reuses the compute client so the admin views read the
// same live OpenStack status as the user-facing compute views.
type liveInstanceStatusSource struct {
	client *compute.Client
}

func NewLiveInstanceStatusSource(provider *gophercloud.ProviderClient) instanceStatusSource {
	return &liveInstanceStatusSource{client: compute.NewClient(provider)}
}

func (s *liveInstanceStatusSource) InstanceStatuses(context.Context) (map[string]string, error) {
	servers, err := s.client.FetchInstances()
	if err != nil {
		return nil, err
	}
	statuses := make(map[string]string, len(servers))
	for _, srv := range servers {
		statuses[srv.ID] = srv.Status
	}
	return statuses, nil
}

func (s *liveInstanceStatusSource) InstanceStatus(_ context.Context, id string) (string, error) {
	srv, err := s.client.FetchInstance(id)
	if err != nil {
		return "", err
	}
	return srv.Status, nil
}
