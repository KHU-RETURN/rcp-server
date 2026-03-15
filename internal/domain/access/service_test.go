package access

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMz7v3R7iK4WbG2ZrM8Z8vV7n6lYx4l6Wwq8m7M+v7gL test@example"

type fakeRepository struct {
	getComputeClientFn func() (*gophercloud.ServiceClient, error)
	getKeyPairFn       func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error)
	createKeyPairFn    func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error)
}

func (f *fakeRepository) GetComputeClient() (*gophercloud.ServiceClient, error) {
	return f.getComputeClientFn()
}

func (f *fakeRepository) GetKeyPair(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
	return f.getKeyPairFn(client, name)
}

func (f *fakeRepository) CreateKeyPair(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
	return f.createKeyPairFn(client, name, publicKey)
}

func newStatusError(code int, body string) gophercloud.ErrUnexpectedResponseCode {
	return gophercloud.ErrUnexpectedResponseCode{
		Method:   "POST",
		URL:      "https://openstack.example/keypairs",
		Expected: []int{http.StatusCreated},
		Actual:   code,
		Body:     []byte(body),
	}
}

func TestServiceCreateKeyPair(t *testing.T) {
	t.Run("creates keypair when input is valid", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				return &keypairs.KeyPair{
					Name:        name,
					Fingerprint: "fingerprint",
					PublicKey:   publicKey,
					PrivateKey:  "must-not-leak",
				}, nil
			},
		}

		svc := NewService(repo)
		res, err := svc.CreateKeyPair(CreateKeyPairRequest{
			Name:      " team-default-key ",
			PublicKey: " " + testPublicKey + " ",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Name != "team-default-key" {
			t.Fatalf("expected trimmed name, got %q", res.Name)
		}
		if res.PublicKey != testPublicKey {
			t.Fatalf("expected trimmed public key, got %q", res.PublicKey)
		}
		if res.Fingerprint != "fingerprint" {
			t.Fatalf("expected fingerprint to be mapped, got %q", res.Fingerprint)
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "   ", PublicKey: testPublicKey})
		if !errors.Is(err, ErrNameRequired) {
			t.Fatalf("expected ErrNameRequired, got %v", err)
		}
	})

	t.Run("rejects empty public key", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: "   "})
		if !errors.Is(err, ErrPublicKeyRequired) {
			t.Fatalf("expected ErrPublicKeyRequired, got %v", err)
		}
	})

	t.Run("rejects invalid public key format", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: "not-a-key"})
		if !errors.Is(err, ErrInvalidSSHKeyFormat) {
			t.Fatalf("expected ErrInvalidSSHKeyFormat, got %v", err)
		}
	})

	t.Run("returns conflict when keypair already exists", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return &keypairs.KeyPair{Name: name}, nil
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				t.Fatal("create should not be called when keypair exists")
				return nil, nil
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairAlreadyExists) {
			t.Fatalf("expected ErrKeyPairAlreadyExists, got %v", err)
		}
	})

	t.Run("normalizes create conflict as conflict error", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault409{}
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairAlreadyExists) {
			t.Fatalf("expected ErrKeyPairAlreadyExists, got %v", err)
		}
	})

	t.Run("normalizes compute client failures as internal operation error", func(t *testing.T) {
		repoErr := errors.New("provider bootstrap leaked")
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return nil, repoErr
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})

	t.Run("normalizes upstream forbidden on lookup", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault403{ErrUnexpectedResponseCode: newStatusError(http.StatusForbidden, "provider-secret")}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				t.Fatal("create should not be called when lookup fails")
				return nil, nil
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairAccessDenied) {
			t.Fatalf("expected ErrKeyPairAccessDenied, got %v", err)
		}
	})

	t.Run("normalizes upstream bad request on create", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault400{ErrUnexpectedResponseCode: newStatusError(http.StatusBadRequest, "provider-secret")}
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrInvalidKeyPairRequest) {
			t.Fatalf("expected ErrInvalidKeyPairRequest, got %v", err)
		}
	})

	t.Run("normalizes upstream server error on create", func(t *testing.T) {
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault500{ErrUnexpectedResponseCode: newStatusError(http.StatusInternalServerError, "provider-secret")}
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})

	t.Run("normalizes generic repository errors as internal operation error", func(t *testing.T) {
		repoErr := errors.New("repository failed")
		repo := &fakeRepository{
			getComputeClientFn: func() (*gophercloud.ServiceClient, error) {
				return &gophercloud.ServiceClient{}, nil
			},
			getKeyPairFn: func(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error) {
				return nil, gophercloud.ErrDefault404{}
			},
			createKeyPairFn: func(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error) {
				return nil, repoErr
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})
}
