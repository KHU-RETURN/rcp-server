package compute

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
)

func TestHandlerGetFlavors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(client *fakeClient) *Handler {
		return NewHandler(newTestService(client))
	}
	newRouter := func(handler *Handler) *gin.Engine {
		r := gin.New()
		v1 := r.Group(api.BasePath)
		v1.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyUser, &auth.User{ID: computeTestUserID})
			c.Next()
		})
		handler.InitRoutes(v1)
		return r
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
		r := newRouter(newHandler(client))
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
		r := newRouter(newHandler(client))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestHandlerGetAvailableFlavors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(client *fakeClient) *Handler {
		return NewHandler(newTestService(client))
	}
	newRouter := func(handler *Handler) *gin.Engine {
		r := gin.New()
		v1 := r.Group(api.BasePath)
		v1.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyUser, &auth.User{ID: computeTestUserID})
			c.Next()
		})
		handler.InitRoutes(v1)
		return r
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
		r := newRouter(newHandler(client))
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
		r := newRouter(newHandler(client))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
	})
}

func TestHandlerCreateServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(repo *fakeClient) *Handler {
		return NewHandler(newTestService(repo))
	}
	newRouter := func(handler *Handler) *gin.Engine {
		r := gin.New()
		v1 := r.Group(api.BasePath)
		v1.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyUser, &auth.User{ID: computeTestUserID})
			c.Next()
		})
		handler.InitRoutes(v1)
		return r
	}

	t.Run("returns 201 with expanded create response", func(t *testing.T) {
		repo := &fakeClient{
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
		r := newRouter(newHandler(repo))
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
		r := newRouter(newHandler(&fakeClient{}))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("returns 400 for whitespace-only required fields without hitting cloud", func(t *testing.T) {
		repo := &fakeClient{
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
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("treats whitespace-only network_id as omitted", func(t *testing.T) {
		repo := &fakeClient{
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
		r := newRouter(newHandler(repo))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}
	})
}
