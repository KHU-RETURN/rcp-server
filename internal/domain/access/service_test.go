package access

import (
	"errors"
	"net/http"
	"testing"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMz7v3R7iK4WbG2ZrM8Z8vV7n6lYx4l6Wwq8m7M+v7gL test@example"

type fakeClient struct {
	listKeyPairsFn  func() ([]KeyPair, error)
	getKeyPairFn    func(name string) (*KeyPair, error)
	createKeyPairFn func(name, publicKey string) (*KeyPair, error)
	deleteKeyPairFn func(name string) error
}

func (f *fakeClient) ListKeyPairs() ([]KeyPair, error) {
	if f.listKeyPairsFn != nil {
		return f.listKeyPairsFn()
	}
	return nil, nil
}

func (f *fakeClient) GetKeyPair(name string) (*KeyPair, error) {
	if f.getKeyPairFn != nil {
		return f.getKeyPairFn(name)
	}
	return nil, nil
}

func (f *fakeClient) CreateKeyPair(name, publicKey string) (*KeyPair, error) {
	if f.createKeyPairFn != nil {
		return f.createKeyPairFn(name, publicKey)
	}
	return nil, nil
}

func (f *fakeClient) DeleteKeyPair(name string) error {
	if f.deleteKeyPairFn != nil {
		return f.deleteKeyPairFn(name)
	}
	return nil
}

func newStatusErr(code int) *StatusError {
	return &StatusError{Code: code, Err: errors.New(http.StatusText(code))}
}

func TestServiceCreateKeyPair(t *testing.T) {
	t.Run("creates keypair when input is valid", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return &KeyPair{
					Name:        name,
					Fingerprint: "fingerprint",
					PublicKey:   publicKey,
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
		svc := NewService(&fakeClient{})
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "   ", PublicKey: testPublicKey})
		if !errors.Is(err, ErrNameRequired) {
			t.Fatalf("expected ErrNameRequired, got %v", err)
		}
	})

	t.Run("rejects empty public key", func(t *testing.T) {
		svc := NewService(&fakeClient{})
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: "   "})
		if !errors.Is(err, ErrPublicKeyRequired) {
			t.Fatalf("expected ErrPublicKeyRequired, got %v", err)
		}
	})

	t.Run("rejects invalid public key format", func(t *testing.T) {
		svc := NewService(&fakeClient{})
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: "not-a-key"})
		if !errors.Is(err, ErrInvalidSSHKeyFormat) {
			t.Fatalf("expected ErrInvalidSSHKeyFormat, got %v", err)
		}
	})

	t.Run("returns conflict when keypair already exists", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return &KeyPair{Name: name}, nil
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
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
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusConflict)
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairAlreadyExists) {
			t.Fatalf("expected ErrKeyPairAlreadyExists, got %v", err)
		}
	})

	t.Run("normalizes upstream forbidden on lookup", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusForbidden)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
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
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusBadRequest)
			},
		}

		svc := NewService(repo)
		_, err := svc.CreateKeyPair(CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrInvalidKeyPairRequest) {
			t.Fatalf("expected ErrInvalidKeyPairRequest, got %v", err)
		}
	})

	t.Run("normalizes upstream server error on create", func(t *testing.T) {
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusInternalServerError)
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
		repo := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
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
