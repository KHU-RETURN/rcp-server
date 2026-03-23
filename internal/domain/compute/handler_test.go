package compute

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/api"
	"github.com/gin-gonic/gin"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type fakeRepository struct {
	fetchFlavorsFn     func() ([]flavors.Flavor, error)
	getComputeQuotaFn  func(client *gophercloud.ServiceClient, projectID string) (*quotasets.QuotaDetailSet, error)
	getComputeClientFn func() (*gophercloud.ServiceClient, error)
	createServerFn     func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error)
}

func (f *fakeRepository) FetchFlavors() ([]flavors.Flavor, error) {
	return f.fetchFlavorsFn()
}

func (f *fakeRepository) GetComputeQuota(client *gophercloud.ServiceClient, projectID string) (*quotasets.QuotaDetailSet, error) {
	return f.getComputeQuotaFn(client, projectID)
}

func (f *fakeRepository) GetComputeClient() (*gophercloud.ServiceClient, error) {
	return f.getComputeClientFn()
}

func (f *fakeRepository) CreateServer(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
	return f.createServerFn(client, opts)
}

func TestHandlerCreateServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(repo *fakeRepository) *Handler {
		return NewHandler(NewService(repo))
	}

	t.Run("returns 201 with legacy server JSON shape", func(t *testing.T) {
		tags := []string{"web"}
		serverGroups := []string{"group-1"}
		server := &servers.Server{
			ID:         "server-uuid",
			TenantID:   "tenant-uuid",
			UserID:     "user-uuid",
			Name:       "team-vm-01",
			Updated:    time.Date(2026, time.March, 23, 1, 2, 3, 0, time.UTC),
			Created:    time.Date(2026, time.March, 23, 1, 0, 0, 0, time.UTC),
			HostID:     "host-id",
			Status:     "BUILD",
			Progress:   12,
			AccessIPv4: "203.0.113.10",
			AccessIPv6: "2001:db8::1",
			Flavor: map[string]any{
				"id": "flavor-uuid",
			},
			Addresses: map[string]any{
				"private": []any{
					map[string]any{
						"addr":    "10.0.0.8",
						"version": float64(4),
					},
				},
			},
			Metadata: map[string]string{
				"env": "dev",
			},
			Links: []interface{}{
				map[string]any{
					"href": "https://example.com/server-uuid",
					"rel":  "self",
				},
			},
			KeyName: "team-default-key",
			SecurityGroups: []map[string]any{
				{"name": "default"},
			},
			AttachedVolumes: []servers.AttachedVolume{
				{ID: "vol-1"},
			},
			Fault: servers.Fault{
				Code:    500,
				Created: time.Date(2026, time.March, 23, 1, 3, 0, 0, time.UTC),
				Details: "detail",
				Message: "err",
			},
			Tags:         &tags,
			ServerGroups: &serverGroups,
		}

		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				if opts.Name != "team-vm-01" || opts.ImageRef != "image-uuid" || opts.FlavorRef != "flavor-uuid" {
					t.Fatalf("unexpected create opts: %+v", opts)
				}
				if len(opts.Networks) != 1 || opts.Networks[0].UUID != "network-uuid" {
					t.Fatalf("expected network UUID to be preserved, got %+v", opts.Networks)
				}
				return server, nil
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:      "team-vm-01",
			ImageRef:  "image-uuid",
			FlavorRef: "flavor-uuid",
			NetworkID: "network-uuid",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}

		expectedJSON, err := json.Marshal(server)
		if err != nil {
			t.Fatalf("failed to marshal expected server JSON: %v", err)
		}
		assertJSONEqual(t, expectedJSON, w.Body.Bytes())
	})

	t.Run("omits networks when network_id is absent", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				if len(opts.Networks) != 0 {
					t.Fatalf("expected no networks, got %+v", opts.Networks)
				}
				return &servers.Server{}, nil
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:      "team-vm-01",
			ImageRef:  "image-uuid",
			FlavorRef: "flavor-uuid",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}
	})

	t.Run("returns 400 for invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compute/instances", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(&fakeRepository{}).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}

		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "Invalid request body" {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})

	t.Run("returns 500 when compute client cannot be created", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return nil, errors.New("bootstrap leaked")
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:      "team-vm-01",
			ImageRef:  "image-uuid",
			FlavorRef: "flavor-uuid",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}

		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "Failed to connect to cloud" {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})

	t.Run("returns 500 with upstream create error message", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
				return nil, errors.New("upstream create server error")
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:      "team-vm-01",
			ImageRef:  "image-uuid",
			FlavorRef: "flavor-uuid",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}

		var res api.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal error response: %v", err)
		}
		if res.Error != "upstream create server error" {
			t.Fatalf("unexpected error response: %+v", res)
		}
	})
}

func TestHandlerGetFlavorsReturnsNamedErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &fakeRepository{
		fetchFlavorsFn: func() ([]flavors.Flavor, error) {
			return nil, errors.New("upstream error")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compute/flavors", nil)
	w := httptest.NewRecorder()
	r := gin.New()
	v1 := r.Group("/api/v1")
	NewHandler(NewService(repo)).InitRoutes(v1)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var res api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}
	if res.Error != "사양 조회를 실패했습니다: upstream error" {
		t.Fatalf("unexpected error response: %+v", res)
	}
}

func assertJSONEqual(t *testing.T, expected, actual []byte) {
	t.Helper()

	var expectedValue any
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatalf("failed to unmarshal expected JSON: %v", err)
	}

	var actualValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("failed to unmarshal actual JSON: %v", err)
	}

	if !reflect.DeepEqual(expectedValue, actualValue) {
		t.Fatalf("expected JSON %s, got %s", string(expected), string(actual))
	}
}
