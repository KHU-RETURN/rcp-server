package access

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
)

func TestHandlerCreateKeyPair(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newHandler := func(repo *fakeRepository) *Handler {
		return NewHandler(NewService(repo))
	}

	t.Run("returns 201 with response body", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				return &keypairs.KeyPair{Name: name, Fingerprint: "fp", PublicKey: publicKey}, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d", w.Code)
		}

		var res KeyPairResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if res.Name != "key" || res.Fingerprint != "fp" || res.PublicKey != testPublicKey {
			t.Fatalf("unexpected response body: %+v", res)
		}
	})

	t.Run("returns 400 for invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/access/keypairs", bytes.NewBufferString("{"))
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

	t.Run("returns 409 for duplicate name", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return &keypairs.KeyPair{Name: name}, nil
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				return nil, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected status 409, got %d", w.Code)
		}
	})

	t.Run("returns 403 with sanitized message for upstream access denied", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault403{
					ErrUnexpectedResponseCode: newStatusError(http.StatusForbidden, "provider-secret"),
				}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				t.Fatal("create should not be called when lookup is forbidden")
				return nil, nil
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "provider-secret") {
			t.Fatalf("response leaked provider details: %s", w.Body.String())
		}
	})

	t.Run("returns 500 with sanitized message for internal failures", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return nil, errors.New("provider bootstrap leaked")
			},
		}

		body, _ := json.Marshal(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/access/keypairs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r := gin.New()
		v1 := r.Group("/api/v1")
		newHandler(repo).InitRoutes(v1)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "provider bootstrap leaked") {
			t.Fatalf("response leaked provider details: %s", w.Body.String())
		}
	})
}
