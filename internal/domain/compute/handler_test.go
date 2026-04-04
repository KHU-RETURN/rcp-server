package compute

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

func TestHandlerCreateServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(repo *fakeRepository) *Handler {
		return NewHandler(NewService(repo))
	}

	t.Run("returns 201 with expanded create response", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
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
	})

	t.Run("returns 400 for whitespace-only required fields without hitting cloud", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				t.Fatal("GetComputeClient should not be called for invalid input")
				return nil, nil
			},
		}

		body, _ := json.Marshal(CreateInstanceRequest{
			Name:      "   ",
			ImageID:   "image-1",
			FlavorID:  "flavor-1",
			NetworkID: "   ",
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/compute/instances", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("treats whitespace-only network_id as omitted", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			createServerFn: func(client *gophercloud.ServiceClient, opts CreateServerOpts) (*servers.Server, error) {
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
}
