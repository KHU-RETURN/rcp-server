package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
)

var testUser = &auth.User{ID: testOwnerID}

func withTestUser(r *gin.RouterGroup) {
	r.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyUser, testUser)
		c.Next()
	})
}

func TestHandlerGetFlavors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(client *fakeClient) *Handler {
		return NewHandler(NewService(client, &fakeRepo{}, "project-1"))
	}

	t.Run("returns 200 with flavor list", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return []Flavor{
					{ID: "1", Name: "m1.small", VCPUs: 1, RAM: 2048, Disk: 20},
				}, nil
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/compute/flavors", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(client).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var res []FlavorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if len(res) != 1 || res[0].ID != "1" || res[0].VCPUs != 1 {
			t.Fatalf("unexpected flavor response: %+v", res)
		}
	})

	t.Run("returns 500 when client fails", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return nil, errors.New("upstream error")
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/compute/flavors", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(client).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestHandlerGetAvailableFlavors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(client *fakeClient) *Handler {
		return NewHandler(NewService(client, &fakeRepo{}, "project-1"))
	}

	t.Run("returns 200 with available flavor list", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return []Flavor{
					{ID: "1", Name: "m1.small", VCPUs: 2, RAM: 1024, Disk: 10},
				}, nil
			},
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return &QuotaDetailSet{
					Cores:     QuotaDetail{Limit: 10, InUse: 4},
					RAM:       QuotaDetail{Limit: 10000, InUse: 0},
					Instances: QuotaDetail{Limit: 10, InUse: 0},
				}, nil
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/compute/flavors?available=true", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(client).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var res []AvailableFlavorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 flavor, got %d", len(res))
		}
		// 6 cores / 2 vcpus = 3
		if res[0].MaxConfigurable != 3 {
			t.Fatalf("expected MaxConfigurable=3, got %d", res[0].MaxConfigurable)
		}
	})

	t.Run("returns 500 when quota fetch fails", func(t *testing.T) {
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return []Flavor{{ID: "1", Name: "m1.small", VCPUs: 2, RAM: 1024}}, nil
			},
			getComputeQuotaFn: func(projectID string) (*QuotaDetailSet, error) {
				return nil, errors.New("quota error")
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/compute/flavors?available=true", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(client).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestHandlerCreateServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(client *fakeClient) *Handler {
		return NewHandler(NewService(client, &fakeRepo{}, "project-1"))
	}

	t.Run("returns 401 when user is not authenticated", func(t *testing.T) {
		body, _ := json.Marshal(CreateInstanceRequest{
			Name:     "test-vm",
			ImageID:  "image-1",
			FlavorID: "flavor-1",
		})

		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(&fakeClient{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("returns 201 with expanded create response", func(t *testing.T) {
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				if opts.Name != "test-vm" {
					t.Fatalf("expected name test-vm, got %q", opts.Name)
				}
				if opts.ImageRef != "image-1" || opts.FlavorRef != "flavor-1" {
					t.Fatalf("unexpected image/flavor refs: %+v", opts)
				}
				if opts.KeyName != "team-key" {
					t.Fatalf("expected key_name team-key, got %q", opts.KeyName)
				}
				if len(opts.SecurityGroups) != 2 || opts.SecurityGroups[0] != "default" || opts.SecurityGroups[1] != "ssh" {
					t.Fatalf("unexpected security_groups: %#v", opts.SecurityGroups)
				}
				if len(opts.Networks) != 1 || opts.Networks[0].UUID != "network-1" {
					t.Fatalf("unexpected networks: %#v", opts.Networks)
				}

				return testServer(map[string]any{
					"private": []any{
						map[string]any{"addr": "10.0.0.8", "OS-EXT-IPS:type": "fixed"},
						map[string]any{"addr": "203.0.113.10", "OS-EXT-IPS:type": "floating"},
					},
				}), nil
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:           "test-vm",
			ImageID:        "image-1",
			FlavorID:       "flavor-1",
			NetworkID:      "network-1",
			KeyName:        "team-key",
			SecurityGroups: []string{"default", "ssh"},
		})

		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(client).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}

		var res CreateInstanceResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if res.KeyName != "team-key" {
			t.Fatalf("expected key_name team-key, got %q", res.KeyName)
		}
		if res.FixedIP != "10.0.0.8" || res.FloatingIP != "203.0.113.10" {
			t.Fatalf("unexpected IPs in response: %+v", res)
		}
	})

	t.Run("returns 400 for invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/compute/instances", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(&fakeClient{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("returns 400 for whitespace-only required fields without hitting cloud", func(t *testing.T) {
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				t.Fatal("CreateServer should not be called for invalid input")
				return nil, nil
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:      "   ",
			ImageID:   "image-1",
			FlavorID:  "flavor-1",
			NetworkID: "   ",
		})

		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(client).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("treats whitespace-only network_id as omitted", func(t *testing.T) {
		client := &fakeClient{
			createServerFn: func(opts CreateServerOpts) (*Server, error) {
				if len(opts.Networks) != 0 {
					t.Fatalf("expected whitespace-only network_id to be omitted, got %#v", opts.Networks)
				}
				return testServer(map[string]any{}), nil
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:      "test-vm",
			ImageID:   "image-1",
			FlavorID:  "flavor-1",
			NetworkID: "   ",
		})

		req := httptest.NewRequest(http.MethodPost, api.BasePath+"/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(client).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}
	})
}

func TestHandlerDeleteInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(client *fakeClient, repo *fakeRepo) *Handler {
		return NewHandler(NewService(client, repo, "project-1"))
	}

	t.Run("returns 401 when user is not authenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/compute/instances/server-1", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(&fakeClient{}, &fakeRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("returns 404 when instance not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/compute/instances/missing", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(&fakeClient{}, &fakeRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", w.Code)
		}
	})

	t.Run("returns 204 on success", func(t *testing.T) {
		repo := &fakeRepo{
			findByOpenstackIDFn: func(_ context.Context, _ uuid.UUID, id string) (*Instance, error) {
				return &Instance{OpenstackID: id}, nil
			},
		}

		req := httptest.NewRequest(http.MethodDelete, api.BasePath+"/compute/instances/server-1", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(&fakeClient{}, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d", w.Code)
		}
	})
}

func TestHandlerGetInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(client *fakeClient, repo *fakeRepo) *Handler {
		return NewHandler(NewService(client, repo, "project-1"))
	}

	t.Run("returns 401 when user is not authenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/compute/instances", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		newHandler(&fakeClient{}, &fakeRepo{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("returns 200 with status and IPs from OpenStack merged with DB metadata", func(t *testing.T) {
		repo := &fakeRepo{
			listByOwnerFn: func(_ context.Context, _ uuid.UUID) ([]Instance, error) {
				return []Instance{
					{OpenstackID: "s-1", Name: "vm-1", ImageID: "img-1", FlavorID: "f-1"},
				}, nil
			},
		}
		client := &fakeClient{
			fetchFlavorsFn: func() ([]Flavor, error) {
				return []Flavor{{ID: "f-1", Name: "m1.small", VCPUs: 1, RAM: 1024}}, nil
			},
			fetchInstancesFn: func() ([]Server, error) {
				return []Server{
					{
						ID:     "s-1",
						Status: "ACTIVE",
						Addresses: map[string]any{
							"private": []any{
								map[string]any{"addr": "10.0.0.1", "OS-EXT-IPS:type": "fixed"},
							},
						},
					},
				}, nil
			},
		}

		req := httptest.NewRequest(http.MethodGet, api.BasePath+"/compute/instances", nil)
		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group(api.BasePath)
		withTestUser(v1)
		newHandler(client, repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var res []InstanceDetailResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if len(res) != 1 {
			t.Fatalf("expected 1 instance, got %d", len(res))
		}
		if res[0].ID != "s-1" || res[0].Status != "ACTIVE" || res[0].FixedIP != "10.0.0.1" {
			t.Fatalf("unexpected response: %+v", res[0])
		}
		if res[0].Flavor.Name != "m1.small" {
			t.Fatalf("expected flavor m1.small, got %q", res[0].Flavor.Name)
		}
	})
}
