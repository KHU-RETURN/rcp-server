package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
)

type fakeComputeStatusClient struct {
	servers    []compute.Server
	fetchCalls int
}

func (f *fakeComputeStatusClient) FetchInstances() ([]compute.Server, error) {
	f.fetchCalls++
	return f.servers, nil
}

func (f *fakeComputeStatusClient) FetchInstance(string) (*compute.Server, error) {
	return nil, errors.New("not implemented")
}

func TestLiveInstanceStatusSourceCachesStatuses(t *testing.T) {
	ctx := context.Background()
	fake := &fakeComputeStatusClient{servers: []compute.Server{{ID: "vm-1", Status: "ACTIVE"}}}
	src := &liveInstanceStatusSource{client: fake}

	for i := 0; i < 3; i++ {
		statuses, err := src.InstanceStatuses(ctx)
		if err != nil {
			t.Fatalf("InstanceStatuses: %v", err)
		}
		if statuses["vm-1"] != "ACTIVE" {
			t.Fatalf("unexpected statuses: %+v", statuses)
		}
	}
	if fake.fetchCalls != 1 {
		t.Fatalf("expected 1 fetch while cached, got %d", fake.fetchCalls)
	}

	// After the TTL expires the source must hit the client again.
	src.expire = time.Now().Add(-time.Second)
	if _, err := src.InstanceStatuses(ctx); err != nil {
		t.Fatalf("InstanceStatuses after expiry: %v", err)
	}
	if fake.fetchCalls != 2 {
		t.Fatalf("expected refresh after expiry, got %d fetches", fake.fetchCalls)
	}
}
