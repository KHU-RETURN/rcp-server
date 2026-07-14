package compute

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gophercloud/gophercloud"
)

var testOwnerID = uuid.New()

type fakeClient struct {
	fetchFlavorsFn     func() ([]Flavor, error)
	getComputeQuotaFn  func(projectID string) (*QuotaDetailSet, error)
	createServerFn     func(opts CreateServerOpts) (*Server, error)
	updateServerNameFn func(id string, name string) (*Server, error)
	pauseServerFn      func(id string) error
	unpauseServerFn    func(id string) error
	deleteServerFn     func(id string) error
	fetchInstancesFn   func() ([]Server, error)
	fetchInstanceFn    func(id string) (*Server, error)
	fetchDiagnosticsFn func(id string) (map[string]any, error)
}

func (f *fakeClient) FetchFlavors() ([]Flavor, error) {
	if f.fetchFlavorsFn != nil {
		return f.fetchFlavorsFn()
	}
	return nil, nil
}

func (f *fakeClient) GetComputeQuota(projectID string) (*QuotaDetailSet, error) {
	if f.getComputeQuotaFn != nil {
		return f.getComputeQuotaFn(projectID)
	}
	return nil, nil
}

func (f *fakeClient) CreateServer(opts CreateServerOpts) (*Server, error) {
	if f.createServerFn != nil {
		return f.createServerFn(opts)
	}
	return nil, nil
}

func (f *fakeClient) UpdateServerName(id string, name string) (*Server, error) {
	if f.updateServerNameFn != nil {
		return f.updateServerNameFn(id, name)
	}
	return &Server{ID: id, Name: name}, nil
}

func (f *fakeClient) PauseServer(id string) error {
	if f.pauseServerFn != nil {
		return f.pauseServerFn(id)
	}
	return nil
}

func (f *fakeClient) UnpauseServer(id string) error {
	if f.unpauseServerFn != nil {
		return f.unpauseServerFn(id)
	}
	return nil
}

func (f *fakeClient) DeleteServer(id string) error {
	if f.deleteServerFn != nil {
		return f.deleteServerFn(id)
	}
	return nil
}

func (f *fakeClient) FetchInstances() ([]Server, error) {
	if f.fetchInstancesFn != nil {
		return f.fetchInstancesFn()
	}
	return nil, nil
}

func (f *fakeClient) FetchInstance(id string) (*Server, error) {
	if f.fetchInstanceFn != nil {
		return f.fetchInstanceFn(id)
	}
	return &Server{ID: id}, nil
}

func (f *fakeClient) FetchDiagnostics(id string) (map[string]any, error) {
	if f.fetchDiagnosticsFn != nil {
		return f.fetchDiagnosticsFn(id)
	}
	return nil, nil
}

type fakeRepo struct {
	saveInstanceFn        func(ctx context.Context, ownerID uuid.UUID, inst *Instance) error
	deleteByOpenstackIDFn func(ctx context.Context, ownerID uuid.UUID, openstackID string) error
	updateMetadataFn      func(ctx context.Context, ownerID uuid.UUID, openstackID string, update UpdateInstanceRequest) error
	listByOwnerFn         func(ctx context.Context, ownerID uuid.UUID) ([]Instance, error)
	findByOpenstackIDFn   func(ctx context.Context, ownerID uuid.UUID, openstackID string) (*Instance, error)
}

func (r *fakeRepo) SaveInstance(ctx context.Context, ownerID uuid.UUID, inst *Instance) error {
	if r.saveInstanceFn != nil {
		return r.saveInstanceFn(ctx, ownerID, inst)
	}
	return nil
}

func (r *fakeRepo) DeleteByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) error {
	if r.deleteByOpenstackIDFn != nil {
		return r.deleteByOpenstackIDFn(ctx, ownerID, openstackID)
	}
	return nil
}

func (r *fakeRepo) UpdateInstanceMetadata(ctx context.Context, ownerID uuid.UUID, openstackID string, update UpdateInstanceRequest) error {
	if r.updateMetadataFn != nil {
		return r.updateMetadataFn(ctx, ownerID, openstackID, update)
	}
	return nil
}

func (r *fakeRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Instance, error) {
	if r.listByOwnerFn != nil {
		return r.listByOwnerFn(ctx, ownerID)
	}
	return nil, nil
}

func (r *fakeRepo) FindByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) (*Instance, error) {
	if r.findByOpenstackIDFn != nil {
		return r.findByOpenstackIDFn(ctx, ownerID, openstackID)
	}
	return nil, nil
}

func TestServiceGetFlavors(t *testing.T) {
	t.Run("returns converted flavors", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return []Flavor{
					{ID: "1", Name: "m1.small", VCPUs: 1, RAM: 2048, Disk: 20},
					{ID: "2", Name: "m1.medium", VCPUs: 2, RAM: 4096, Disk: 40},
				}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.GetFlavors()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("expected 2 flavors, got %d", len(res))
		}
		if res[0].ID != "1" || res[0].VCPUs != 1 || res[0].RAM != 2048 {
			t.Fatalf("unexpected first flavor: %+v", res[0])
		}
		if res[1].ID != "2" || res[1].Name != "m1.medium" {
			t.Fatalf("unexpected second flavor: %+v", res[1])
		}
	})

	t.Run("returns empty slice when no flavors", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) { return []Flavor{}, nil },
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.GetFlavors()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Fatalf("expected empty slice, got %v", res)
		}
	})

	t.Run("propagates client error", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return nil, errors.New("upstream error")
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		_, err := svc.GetFlavors()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("caches flavors across calls", func(t *testing.T) {
		calls := 0
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				calls++
				return []Flavor{{ID: "1", Name: "m1.small"}}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		for range 3 {
			if _, err := svc.GetFlavors(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if calls != 1 {
			t.Fatalf("expected 1 upstream fetch, got %d", calls)
		}
	})

	t.Run("does not cache errors", func(t *testing.T) {
		calls := 0
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				calls++
				if calls == 1 {
					return nil, errors.New("upstream error")
				}
				return []Flavor{{ID: "1"}}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		if _, err := svc.GetFlavors(); err == nil {
			t.Fatal("expected error, got nil")
		}
		res, err := svc.GetFlavors()
		if err != nil || len(res) != 1 {
			t.Fatalf("expected retry to succeed, got res=%v err=%v", res, err)
		}
	})
}

func TestServiceFetchFlavorsConcurrency(t *testing.T) {
	t.Run("cold start: concurrent callers share a single upstream fetch", func(t *testing.T) {
		var calls int32
		started := make(chan struct{})
		var startOnce sync.Once
		release := make(chan struct{})
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				atomic.AddInt32(&calls, 1)
				startOnce.Do(func() { close(started) })
				<-release
				return []Flavor{{ID: "1"}}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")

		const n = 10
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				res, err := svc.GetFlavors()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(res) != 1 || res[0].ID != "1" {
					t.Errorf("unexpected result: %#v", res)
				}
			}()
		}

		<-started
		close(release)
		wg.Wait()

		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected exactly 1 upstream fetch, got %d", got)
		}
	})

	t.Run("stale cache is served immediately while a single background refresh runs", func(t *testing.T) {
		var calls int32
		release := make(chan struct{})
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				if atomic.AddInt32(&calls, 1) == 1 {
					return []Flavor{{ID: "stale"}}, nil
				}
				<-release // 백그라운드 갱신 호출: 검증이 끝날 때까지 대기
				return []Flavor{{ID: "fresh"}}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")

		if _, err := svc.GetFlavors(); err != nil {
			t.Fatalf("unexpected error priming cache: %v", err)
		}
		svc.flavorMu.Lock()
		svc.flavorExpire = time.Now().Add(-time.Second)
		svc.flavorMu.Unlock()

		const n = 5
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				res, err := svc.GetFlavors()
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(res) != 1 || res[0].ID != "stale" {
					t.Errorf("expected stale flavor served, got %#v", res)
				}
			}()
		}
		wg.Wait()

		close(release)
		deadline := time.After(time.Second)
		for {
			svc.flavorMu.Lock()
			refreshing := svc.flavorRefreshing
			svc.flavorMu.Unlock()
			if !refreshing {
				break
			}
			select {
			case <-deadline:
				t.Fatal("timed out waiting for background refresh to finish")
			case <-time.After(time.Millisecond):
			}
		}

		if got := atomic.LoadInt32(&calls); got != 2 {
			t.Fatalf("expected exactly 2 upstream fetches (prime + 1 refresh), got %d", got)
		}
	})
}

func TestServiceGetAvailableFlavorsWithLimit(t *testing.T) {
	flavors := []Flavor{
		{ID: "1", Name: "m1.small", VCPUs: 2, RAM: 1024, Disk: 10},
		{ID: "2", Name: "m1.medium", VCPUs: 4, RAM: 2048, Disk: 20},
	}

	t.Run("calculates max based on cpu constraint", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) { return flavors, nil },
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return &QuotaDetailSet{
					Cores:     QuotaDetail{Limit: 8, InUse: 2}, // 6 remaining
					RAM:       QuotaDetail{Limit: 99999, InUse: 0},
					Instances: QuotaDetail{Limit: 100, InUse: 0},
				}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.GetAvailableFlavorsWithLimit()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("expected 2 flavors, got %d", len(res))
		}
		// 6 cores / 2 vcpus = 3 for m1.small
		if res[0].MaxConfigurable != 3 {
			t.Fatalf("expected MaxConfigurable=3 for m1.small, got %d", res[0].MaxConfigurable)
		}
		// 6 cores / 4 vcpus = 1 for m1.medium
		if res[1].MaxConfigurable != 1 {
			t.Fatalf("expected MaxConfigurable=1 for m1.medium, got %d", res[1].MaxConfigurable)
		}
	})

	t.Run("calculates max based on ram constraint", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) { return flavors, nil },
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return &QuotaDetailSet{
					Cores:     QuotaDetail{Limit: 99999, InUse: 0},
					RAM:       QuotaDetail{Limit: 3000, InUse: 0}, // 3000 MB remaining
					Instances: QuotaDetail{Limit: 100, InUse: 0},
				}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.GetAvailableFlavorsWithLimit()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 3000 / 1024 = 2 for m1.small
		if res[0].MaxConfigurable != 2 {
			t.Fatalf("expected MaxConfigurable=2 for m1.small, got %d", res[0].MaxConfigurable)
		}
		// 3000 / 2048 = 1 for m1.medium
		if res[1].MaxConfigurable != 1 {
			t.Fatalf("expected MaxConfigurable=1 for m1.medium, got %d", res[1].MaxConfigurable)
		}
	})

	t.Run("calculates max based on instance count constraint", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) { return flavors, nil },
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return &QuotaDetailSet{
					Cores:     QuotaDetail{Limit: 99999, InUse: 0},
					RAM:       QuotaDetail{Limit: 99999, InUse: 0},
					Instances: QuotaDetail{Limit: 3, InUse: 2}, // 1 remaining
				}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.GetAvailableFlavorsWithLimit()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, f := range res {
			if f.MaxConfigurable != 1 {
				t.Fatalf("expected MaxConfigurable=1 for %s, got %d", f.Name, f.MaxConfigurable)
			}
		}
	})

	t.Run("returns 0 when quota is exhausted", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) { return flavors, nil },
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return &QuotaDetailSet{
					Cores:     QuotaDetail{Limit: 10, InUse: 10},
					RAM:       QuotaDetail{Limit: 10000, InUse: 0},
					Instances: QuotaDetail{Limit: 10, InUse: 0},
				}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.GetAvailableFlavorsWithLimit()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, f := range res {
			if f.MaxConfigurable != 0 {
				t.Fatalf("expected MaxConfigurable=0, got %d for %s", f.MaxConfigurable, f.Name)
			}
		}
	})

	t.Run("uses 999 fallback when flavor has zero vcpus", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return []Flavor{{ID: "z", Name: "zero-cpu", VCPUs: 0, RAM: 1024, Disk: 0}}, nil
			},
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return &QuotaDetailSet{
					Cores:     QuotaDetail{Limit: 10, InUse: 5},
					RAM:       QuotaDetail{Limit: 10000, InUse: 0},
					Instances: QuotaDetail{Limit: 5, InUse: 0},
				}, nil
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.GetAvailableFlavorsWithLimit()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// min(instances=5, min(ram-based, 999)) = 5
		if res[0].MaxConfigurable != 5 {
			t.Fatalf("expected MaxConfigurable=5 (instance limit), got %d", res[0].MaxConfigurable)
		}
	})

	t.Run("propagates fetch flavors error", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return nil, errors.New("flavors fetch failed")
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		_, err := svc.GetAvailableFlavorsWithLimit()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("propagates quota error", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) { return flavors, nil },
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return nil, errors.New("quota fetch failed")
			},
		}
		svc := NewService(client, &fakeRepo{}, "project-1", "")
		_, err := svc.GetAvailableFlavorsWithLimit()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestServiceCreateInstance(t *testing.T) {
	ctx := context.Background()

	t.Run("maps floating and fixed IPs into response", func(t *testing.T) {
		var gotOpts CreateServerOpts
		var saved Instance
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				gotOpts = opts
				return testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.8", "OS-EXT-IPS:type": "fixed"},
						map[string]any{"addr": "203.0.113.10", "OS-EXT-IPS:type": "floating"},
					},
				}), nil
			},
		}
		repo := &fakeRepo{
			saveInstanceFn: func(_ context.Context, _ uuid.UUID, inst *Instance) error {
				saved = *inst
				return nil
			},
		}

		svc := NewService(client, repo, "project-1", "")
		res, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
			Name:           " test-vm ",
			ImageRef:       " image-1 ",
			FlavorRef:      " flavor-1 ",
			KeyName:        " team-key ",
			SecurityGroups: []string{" default ", " ", "ssh"},
			Networks:       []NetworkID{{UUID: " network-1 "}},
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
		if saved.Status != "BUILD" {
			t.Fatalf("expected saved status BUILD, got %q", saved.Status)
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
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				return testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.9", "OS-EXT-IPS:type": "fixed"},
					},
				}), nil
			},
		}

		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
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
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				server := testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.10", "OS-EXT-IPS:type": "fixed"},
					},
				})
				server.AccessIPv4 = "198.51.100.24"
				return server, nil
			},
		}

		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
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
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				return &Server{
					ID:        "server-2",
					Name:      "fallback-vm",
					Status:    "BUILD",
					Addresses: map[string]any{},
				}, nil
			},
		}

		svc := NewService(client, &fakeRepo{}, "project-1", "")
		res, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
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
		svc := NewService(&fakeClient{}, &fakeRepo{}, "project-1", "")

		_, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
			Name:      "   ",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
		})
		if err != ErrCreateInstanceNameRequired {
			t.Fatalf("expected ErrCreateInstanceNameRequired, got %v", err)
		}

		_, err = svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "   ",
			FlavorRef: "flavor-1",
		})
		if err != ErrCreateInstanceImageRequired {
			t.Fatalf("expected ErrCreateInstanceImageRequired, got %v", err)
		}

		_, err = svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
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
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				gotOpts = opts
				return testServer(map[string]any{}), nil
			},
		}

		svc := NewService(client, &fakeRepo{}, "project-1", "")
		_, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
			Networks:  []NetworkID{{UUID: "   "}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gotOpts.Networks) != 0 {
			t.Fatalf("expected blank networks to be dropped, got %#v", gotOpts.Networks)
		}
	})

	t.Run("falls back to default network when request omits one", func(t *testing.T) {
		var gotOpts CreateServerOpts
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				gotOpts = opts
				return testServer(map[string]any{}), nil
			},
		}

		svc := NewService(client, &fakeRepo{}, "project-1", "default-net-uuid")
		_, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gotOpts.Networks) != 1 || gotOpts.Networks[0].UUID != "default-net-uuid" {
			t.Fatalf("expected default network fallback, got %#v", gotOpts.Networks)
		}
	})

	t.Run("does not override explicit network with default", func(t *testing.T) {
		var gotOpts CreateServerOpts
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				gotOpts = opts
				return testServer(map[string]any{}), nil
			},
		}

		svc := NewService(client, &fakeRepo{}, "project-1", "default-net-uuid")
		_, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
			Networks:  []NetworkID{{UUID: "explicit-net"}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gotOpts.Networks) != 1 || gotOpts.Networks[0].UUID != "explicit-net" {
			t.Fatalf("expected explicit network preserved, got %#v", gotOpts.Networks)
		}
	})

	t.Run("returns operation failed when repo save errors", func(t *testing.T) {
		var cleanedUpID string
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				return testServer(map[string]any{}), nil
			},
			deleteServerFn: func(id string) error {
				cleanedUpID = id
				return nil
			},
		}
		repo := &fakeRepo{
			saveInstanceFn: func(_ context.Context, _ uuid.UUID, _ *Instance) error {
				return errors.New("db error")
			},
		}

		svc := NewService(client, repo, "project-1", "")
		_, err := svc.CreateInstance(ctx, testOwnerID, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
		})
		if !errors.Is(err, ErrInstanceOperationFailed) {
			t.Fatalf("expected ErrInstanceOperationFailed, got %v", err)
		}
		if cleanedUpID != "server-1" {
			t.Fatalf("expected orphaned server-1 to be cleaned up, got %q", cleanedUpID)
		}
	})
}

func TestServiceGetInstances(t *testing.T) {
	ctx := context.Background()

	t.Run("returns only instances that still exist in OpenStack and prunes stale DB rows", func(t *testing.T) {
		var deletedIDs []string
		repo := &fakeRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Instance, error) {
				return []Instance{
					{OpenstackID: "server-1", Name: "vm-1", ImageID: "image-1", FlavorID: "flavor-1"},
					{OpenstackID: "server-stale", Name: "old-vm", ImageID: "image-2", FlavorID: "flavor-2"},
				}, nil
			},
			deleteByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, id string) error {
				deletedIDs = append(deletedIDs, id)
				return nil
			},
		}
		client := &fakeClient{
			fetchInstancesFn: func() ([]Server, error) {
				return []Server{{
					ID:     "server-1",
					Status: "ACTIVE",
					Addresses: map[string]any{
						"private": []any{
							map[string]any{"addr": "10.0.0.8", "OS-EXT-IPS:type": "fixed"},
						},
					},
				}}, nil
			},
			fetchInstanceFn: func(id string) (*Server, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			fetchFlavorsFn: func() ([]Flavor, error) {
				return []Flavor{
					{ID: "flavor-1", Name: "m1.small", VCPUs: 1, RAM: 1024},
					{ID: "flavor-2", Name: "m1.medium", VCPUs: 2, RAM: 2048},
				}, nil
			},
		}

		svc := NewService(client, repo, "project-1", "")
		res, err := svc.GetInstances(ctx, testOwnerID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 live instance, got %d: %+v", len(res), res)
		}
		if res[0].ID != "server-1" || res[0].Status != "ACTIVE" {
			t.Fatalf("unexpected live instance response: %+v", res[0])
		}
		if !reflect.DeepEqual(deletedIDs, []string{"server-stale"}) {
			t.Fatalf("expected stale server to be deleted, got %#v", deletedIDs)
		}
	})

	t.Run("prune failure is non-fatal and still returns the live instance list", func(t *testing.T) {
		repo := &fakeRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Instance, error) {
				return []Instance{{OpenstackID: "server-stale"}}, nil
			},
			deleteByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, _ string) error {
				return errors.New("db delete failed")
			},
		}
		client := &fakeClient{
			fetchInstancesFn: func() ([]Server, error) { return []Server{}, nil },
			fetchInstanceFn: func(id string) (*Server, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			fetchFlavorsFn: func() ([]Flavor, error) { return []Flavor{}, nil },
		}

		svc := NewService(client, repo, "project-1", "")
		res, err := svc.GetInstances(ctx, testOwnerID)
		if err != nil {
			t.Fatalf("expected prune failure to be non-fatal, got err: %v", err)
		}
		if len(res) != 0 {
			t.Fatalf("expected empty live instance list, got %#v", res)
		}
	})

	t.Run("confirms stale IDs concurrently", func(t *testing.T) {
		staleIDs := []string{"server-a", "server-b", "server-c"}
		repo := &fakeRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Instance, error) {
				instances := make([]Instance, len(staleIDs))
				for i, id := range staleIDs {
					instances[i] = Instance{OpenstackID: id}
				}
				return instances, nil
			},
			deleteByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
		}

		var inFlight int32
		var maxInFlight int32
		release := make(chan struct{})
		client := &fakeClient{
			fetchInstancesFn: func() ([]Server, error) { return []Server{}, nil },
			fetchInstanceFn: func(id string) (*Server, error) {
				cur := atomic.AddInt32(&inFlight, 1)
				for {
					prev := atomic.LoadInt32(&maxInFlight)
					if cur <= prev || atomic.CompareAndSwapInt32(&maxInFlight, prev, cur) {
						break
					}
				}
				<-release
				atomic.AddInt32(&inFlight, -1)
				return nil, gophercloud.ErrDefault404{}
			},
			fetchFlavorsFn: func() ([]Flavor, error) { return []Flavor{}, nil },
		}

		svc := NewService(client, repo, "project-1", "")
		done := make(chan struct{})
		go func() {
			defer close(done)
			if _, err := svc.GetInstances(ctx, testOwnerID); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()

		// 모든 staleID 확인이 동시에 진행 중일 때까지 대기한다.
		deadline := time.After(time.Second)
		for {
			if atomic.LoadInt32(&maxInFlight) == int32(len(staleIDs)) {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("expected %d concurrent FetchInstance calls, max seen %d", len(staleIDs), atomic.LoadInt32(&maxInFlight))
			case <-time.After(time.Millisecond):
			}
		}
		close(release)
		<-done
	})

	t.Run("keeps DB row when server missing from list snapshot but still exists", func(t *testing.T) {
		var deletedIDs []string
		repo := &fakeRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Instance, error) {
				return []Instance{{OpenstackID: "server-racing"}}, nil
			},
			deleteByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, id string) error {
				deletedIDs = append(deletedIDs, id)
				return nil
			},
		}
		client := &fakeClient{
			// 목록 스냅샷에는 없지만 개별 조회로는 살아있는 서버 (생성 직후 레이스)
			fetchInstancesFn: func() ([]Server, error) { return []Server{}, nil },
			fetchInstanceFn: func(id string) (*Server, error) {
				return &Server{ID: id, Status: "BUILD"}, nil
			},
			fetchFlavorsFn: func() ([]Flavor, error) { return []Flavor{}, nil },
		}

		svc := NewService(client, repo, "project-1", "")
		if _, err := svc.GetInstances(ctx, testOwnerID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(deletedIDs) != 0 {
			t.Fatalf("expected no deletion for live server, got %#v", deletedIDs)
		}
	})

	t.Run("keeps DB row when per-ID confirm fails transiently", func(t *testing.T) {
		var deletedIDs []string
		repo := &fakeRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Instance, error) {
				return []Instance{{OpenstackID: "server-x"}}, nil
			},
			deleteByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, id string) error {
				deletedIDs = append(deletedIDs, id)
				return nil
			},
		}
		client := &fakeClient{
			fetchInstancesFn: func() ([]Server, error) { return []Server{}, nil },
			fetchInstanceFn: func(id string) (*Server, error) {
				return nil, errors.New("timeout")
			},
			fetchFlavorsFn: func() ([]Flavor, error) { return []Flavor{}, nil },
		}

		svc := NewService(client, repo, "project-1", "")
		if _, err := svc.GetInstances(ctx, testOwnerID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(deletedIDs) != 0 {
			t.Fatalf("expected no deletion on transient error, got %#v", deletedIDs)
		}
	})
}

func TestServiceUpdateInstance(t *testing.T) {
	ctx := context.Background()
	existing := &Instance{OpenstackID: "server-1", Name: "old-name", KeyName: "my-key", Note: "my-note"}

	t.Run("preserves key_name and note when request only sets name", func(t *testing.T) {
		var captured UpdateInstanceRequest
		repo := &fakeRepo{
			findByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, _ string) (*Instance, error) {
				return existing, nil
			},
			updateMetadataFn: func(_ context.Context, _ uuid.UUID, _ string, update UpdateInstanceRequest) error {
				captured = update
				return nil
			},
		}
		svc := NewService(&fakeClient{}, repo, "project-1", "")
		if _, err := svc.UpdateInstance(ctx, testOwnerID, "server-1", UpdateInstanceRequest{Name: "new-name"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := UpdateInstanceRequest{Name: "new-name", KeyName: "my-key", Note: "my-note"}
		if captured != want {
			t.Fatalf("expected %+v, got %+v", want, captured)
		}
	})

	t.Run("does not touch DB when OpenStack rename fails", func(t *testing.T) {
		dbTouched := false
		repo := &fakeRepo{
			findByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, _ string) (*Instance, error) {
				return existing, nil
			},
			updateMetadataFn: func(_ context.Context, _ uuid.UUID, _ string, _ UpdateInstanceRequest) error {
				dbTouched = true
				return nil
			},
		}
		client := &fakeClient{
			updateServerNameFn: func(id, name string) (*Server, error) {
				return nil, errors.New("nova down")
			},
		}
		svc := NewService(client, repo, "project-1", "")
		_, err := svc.UpdateInstance(ctx, testOwnerID, "server-1", UpdateInstanceRequest{Name: "new-name"})
		if !errors.Is(err, ErrInstanceOperationFailed) {
			t.Fatalf("expected ErrInstanceOperationFailed, got %v", err)
		}
		if dbTouched {
			t.Fatal("expected DB untouched when OpenStack rename fails")
		}
	})
}

func TestServiceDeleteInstance(t *testing.T) {
	ctx := context.Background()

	t.Run("returns not found when instance does not exist in DB", func(t *testing.T) {
		svc := NewService(&fakeClient{}, &fakeRepo{}, "project-1", "")
		err := svc.DeleteInstance(ctx, testOwnerID, "missing-id")
		if !errors.Is(err, ErrInstanceNotFound) {
			t.Fatalf("expected ErrInstanceNotFound, got %v", err)
		}
	})

	t.Run("returns operation failed when repo find errors", func(t *testing.T) {
		repo := &fakeRepo{
			findByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, _ string) (*Instance, error) {
				return nil, errors.New("db error")
			},
		}
		svc := NewService(&fakeClient{}, repo, "project-1", "")
		err := svc.DeleteInstance(ctx, testOwnerID, "some-id")
		if !errors.Is(err, ErrInstanceOperationFailed) {
			t.Fatalf("expected ErrInstanceOperationFailed, got %v", err)
		}
	})

	t.Run("returns operation failed when openstack delete errors", func(t *testing.T) {
		repo := &fakeRepo{
			findByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, id string) (*Instance, error) {
				return &Instance{OpenstackID: id}, nil
			},
		}
		client := &fakeClient{
			deleteServerFn: func(id string) error {
				return errors.New("upstream error")
			},
		}
		svc := NewService(client, repo, "project-1", "")
		err := svc.DeleteInstance(ctx, testOwnerID, "some-id")
		if !errors.Is(err, ErrInstanceOperationFailed) {
			t.Fatalf("expected ErrInstanceOperationFailed, got %v", err)
		}
	})

	t.Run("deletes successfully", func(t *testing.T) {
		var deletedID string
		repo := &fakeRepo{
			findByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, id string) (*Instance, error) {
				return &Instance{OpenstackID: id}, nil
			},
			deleteByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, id string) error {
				deletedID = id
				return nil
			},
		}
		svc := NewService(&fakeClient{}, repo, "project-1", "")
		if err := svc.DeleteInstance(ctx, testOwnerID, "server-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deletedID != "server-1" {
			t.Fatalf("expected deletedID=server-1, got %q", deletedID)
		}
	})
}

func testServer(addresses map[string]any) *Server {
	return &Server{
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
