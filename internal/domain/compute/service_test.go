package compute

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

var computeTestUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

type fakeClient struct {
	fetchFlavorsFn        func() ([]Flavor, error)
	getComputeQuotaFn     func(projectID string) (*QuotaDetailSet, error)
	createServerFn        func(opts CreateServerOpts) (*Server, error)
	fetchInstancesFn      func() ([]Server, error)
	fetchInstanceDetailFn func(id string) (*Server, map[string]any, error)
	deleteServerFn        func(id string) error
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

func (f *fakeClient) FetchInstances() ([]Server, error) {
	if f.fetchInstancesFn != nil {
		return f.fetchInstancesFn()
	}
	return nil, nil
}

func (f *fakeClient) FetchInstanceDetail(id string) (*Server, map[string]any, error) {
	if f.fetchInstanceDetailFn != nil {
		return f.fetchInstanceDetailFn(id)
	}
	return nil, nil, nil
}

func (f *fakeClient) DeleteServer(id string) error {
	if f.deleteServerFn != nil {
		return f.deleteServerFn(id)
	}
	return nil
}

type noopComputeRepository struct {
	saveInstanceFn           func(ctx context.Context, userID uuid.UUID, openstackID, name string) error
	deleteInstanceFn         func(ctx context.Context, userID uuid.UUID, openstackID string) error
	findOpenstackIDsByUserID func(ctx context.Context, userID uuid.UUID) (map[string]string, error)
	isOwnerFn                func(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error)
}

func (r *noopComputeRepository) SaveInstance(ctx context.Context, userID uuid.UUID, openstackID, name string) error {
	if r.saveInstanceFn != nil {
		return r.saveInstanceFn(ctx, userID, openstackID, name)
	}
	return nil
}

func (r *noopComputeRepository) DeleteInstance(ctx context.Context, userID uuid.UUID, openstackID string) error {
	if r.deleteInstanceFn != nil {
		return r.deleteInstanceFn(ctx, userID, openstackID)
	}
	return nil
}

func (r *noopComputeRepository) FindOpenstackIDsByUserID(ctx context.Context, userID uuid.UUID) (map[string]string, error) {
	if r.findOpenstackIDsByUserID != nil {
		return r.findOpenstackIDsByUserID(ctx, userID)
	}
	return map[string]string{}, nil
}

func (r *noopComputeRepository) IsOwner(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error) {
	if r.isOwnerFn != nil {
		return r.isOwnerFn(ctx, userID, openstackID)
	}
	return true, nil
}

func newTestService(client *fakeClient) *Service {
	return NewService(client, "project-1", &noopComputeRepository{})
}

func newTestServiceWithRepo(client *fakeClient, repo computeRepository) *Service {
	return NewService(client, "project-1", repo)
}

func newComputeStatusErr(code int) *StatusError {
	return &StatusError{Code: code, Err: errors.New(http.StatusText(code))}
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
		svc := newTestService(client)
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
		svc := newTestService(client)
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
		svc := newTestService(client)
		_, err := svc.GetFlavors()
		if err == nil {
			t.Fatal("expected error, got nil")
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
		svc := newTestService(client)
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
		svc := newTestService(client)
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
		svc := newTestService(client)
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
		svc := newTestService(client)
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
		svc := newTestService(client)
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
		svc := newTestService(client)
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
		svc := newTestService(client)
		_, err := svc.GetAvailableFlavorsWithLimit()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestServiceCreateInstance(t *testing.T) {
	t.Run("maps floating and fixed IPs into response", func(t *testing.T) {
		var gotOpts CreateServerOpts
		repo := &fakeClient{
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

		ownershipRepo := &noopComputeRepository{
			saveInstanceFn: func(ctx context.Context, userID uuid.UUID, openstackID, name string) error {
				if userID != computeTestUserID {
					t.Fatalf("expected userID %s, got %s", computeTestUserID, userID)
				}
				if openstackID != "server-1" {
					t.Fatalf("expected openstackID server-1, got %q", openstackID)
				}
				if name != "test-vm" {
					t.Fatalf("expected saved name test-vm, got %q", name)
				}
				return nil
			},
		}

		svc := newTestServiceWithRepo(repo, ownershipRepo)
		res, err := svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
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
		repo := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				return testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.9", "OS-EXT-IPS:type": "fixed"},
					},
				}), nil
			},
		}

		svc := newTestService(repo)
		res, err := svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
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
		repo := &fakeClient{
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

		svc := newTestService(repo)
		res, err := svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
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
		repo := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				return &Server{
					ID:        "server-2",
					Name:      "fallback-vm",
					Status:    "BUILD",
					Addresses: map[string]any{},
				}, nil
			},
		}

		svc := newTestService(repo)
		res, err := svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
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
		svc := newTestService(&fakeClient{})

		_, err := svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
			Name:      "   ",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
		})
		if err != ErrCreateInstanceNameRequired {
			t.Fatalf("expected ErrCreateInstanceNameRequired, got %v", err)
		}

		_, err = svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "   ",
			FlavorRef: "flavor-1",
		})
		if err != ErrCreateInstanceImageRequired {
			t.Fatalf("expected ErrCreateInstanceImageRequired, got %v", err)
		}

		_, err = svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
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
		repo := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				gotOpts = opts
				return testServer(map[string]any{}), nil
			},
		}

		svc := newTestService(repo)
		_, err := svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
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

	t.Run("deletes created server when ownership save fails", func(t *testing.T) {
		saveErr := errors.New("db write failed")
		var deletedServerID string
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				return testServer(map[string]any{}), nil
			},
			deleteServerFn: func(id string) error {
				deletedServerID = id
				return nil
			},
		}
		repo := &noopComputeRepository{
			saveInstanceFn: func(ctx context.Context, userID uuid.UUID, openstackID, name string) error {
				return saveErr
			},
		}

		svc := newTestServiceWithRepo(client, repo)
		_, err := svc.CreateInstance(context.Background(), computeTestUserID, CreateServerOpts{
			Name:      "vm",
			ImageRef:  "image-1",
			FlavorRef: "flavor-1",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if deletedServerID != "server-1" {
			t.Fatalf("expected cleanup delete for server-1, got %q", deletedServerID)
		}
		if !errors.Is(err, saveErr) {
			t.Fatalf("expected wrapped save error, got %v", err)
		}
	})
}

func TestServiceGetInstanceDetail(t *testing.T) {
	t.Run("normalizes upstream 404 as instance not found", func(t *testing.T) {
		client := &fakeClient{
			fetchInstanceDetailFn: func(id string) (*Server, map[string]any, error) {
				return nil, nil, newComputeStatusErr(http.StatusNotFound)
			},
		}
		repo := &noopComputeRepository{
			isOwnerFn: func(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error) {
				return true, nil
			},
		}

		svc := newTestServiceWithRepo(client, repo)
		_, err := svc.GetInstanceDetail(context.Background(), computeTestUserID, "server-1")
		if !errors.Is(err, ErrInstanceNotFound) {
			t.Fatalf("expected ErrInstanceNotFound, got %v", err)
		}
	})
}

func TestServiceDeleteInstance(t *testing.T) {
	t.Run("passes user filter to repository delete", func(t *testing.T) {
		client := &fakeClient{
			deleteServerFn: func(id string) error { return nil },
		}
		var gotUserID uuid.UUID
		var gotOpenstackID string
		repo := &noopComputeRepository{
			isOwnerFn: func(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error) {
				return true, nil
			},
			deleteInstanceFn: func(ctx context.Context, userID uuid.UUID, openstackID string) error {
				gotUserID = userID
				gotOpenstackID = openstackID
				return nil
			},
		}

		svc := newTestServiceWithRepo(client, repo)
		if err := svc.DeleteInstance(context.Background(), computeTestUserID, "server-1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotUserID != computeTestUserID {
			t.Fatalf("expected userID %s, got %s", computeTestUserID, gotUserID)
		}
		if gotOpenstackID != "server-1" {
			t.Fatalf("expected openstackID server-1, got %q", gotOpenstackID)
		}
	})

	t.Run("returns nil when stale row delete fails after cloud delete", func(t *testing.T) {
		client := &fakeClient{
			deleteServerFn: func(id string) error { return nil },
		}
		repo := &noopComputeRepository{
			isOwnerFn: func(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error) {
				return true, nil
			},
			deleteInstanceFn: func(ctx context.Context, userID uuid.UUID, openstackID string) error {
				return errors.New("db delete failed")
			},
		}

		svc := newTestServiceWithRepo(client, repo)
		if err := svc.DeleteInstance(context.Background(), computeTestUserID, "server-1"); err != nil {
			t.Fatalf("expected nil on stale row, got %v", err)
		}
	})

	t.Run("normalizes upstream 404 as instance not found", func(t *testing.T) {
		client := &fakeClient{
			deleteServerFn: func(id string) error { return newComputeStatusErr(http.StatusNotFound) },
		}
		repo := &noopComputeRepository{
			isOwnerFn: func(ctx context.Context, userID uuid.UUID, openstackID string) (bool, error) {
				return true, nil
			},
		}

		svc := newTestServiceWithRepo(client, repo)
		err := svc.DeleteInstance(context.Background(), computeTestUserID, "server-1")
		if !errors.Is(err, ErrInstanceNotFound) {
			t.Fatalf("expected ErrInstanceNotFound, got %v", err)
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
