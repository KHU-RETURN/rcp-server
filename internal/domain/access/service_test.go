package access

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMz7v3R7iK4WbG2ZrM8Z8vV7n6lYx4l6Wwq8m7M+v7gL test@example"

var accessTestUserID = uuid.MustParse("22222222-2222-2222-2222-222222222222")

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

type noopKeyPairRepository struct {
	saveKeyPairFn   func(ctx context.Context, userID uuid.UUID, name string) error
	deleteKeyPairFn func(ctx context.Context, userID uuid.UUID, name string) error
	findNamesFn     func(ctx context.Context, userID uuid.UUID) ([]string, error)
	isOwnerFn       func(ctx context.Context, userID uuid.UUID, name string) (bool, error)
}

func (r *noopKeyPairRepository) SaveKeyPair(ctx context.Context, userID uuid.UUID, name string) error {
	if r.saveKeyPairFn != nil {
		return r.saveKeyPairFn(ctx, userID, name)
	}
	return nil
}

func (r *noopKeyPairRepository) DeleteKeyPair(ctx context.Context, userID uuid.UUID, name string) error {
	if r.deleteKeyPairFn != nil {
		return r.deleteKeyPairFn(ctx, userID, name)
	}
	return nil
}

func (r *noopKeyPairRepository) FindNamesByUserID(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if r.findNamesFn != nil {
		return r.findNamesFn(ctx, userID)
	}
	return nil, nil
}

func (r *noopKeyPairRepository) IsOwner(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
	if r.isOwnerFn != nil {
		return r.isOwnerFn(ctx, userID, name)
	}
	return true, nil
}

func newTestService(client *fakeClient) *Service {
	return NewService(client, &noopKeyPairRepository{})
}

func newTestServiceWithRepo(client *fakeClient, repo keypairRepository) *Service {
	return NewService(client, repo)
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

		ownershipRepo := &noopKeyPairRepository{
			saveKeyPairFn: func(ctx context.Context, userID uuid.UUID, name string) error {
				if userID != accessTestUserID {
					t.Fatalf("expected userID %s, got %s", accessTestUserID, userID)
				}
				if name != "team-default-key" {
					t.Fatalf("expected saved keypair name team-default-key, got %q", name)
				}
				return nil
			},
		}

		svc := newTestServiceWithRepo(repo, ownershipRepo)
		res, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{
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
		svc := newTestService(&fakeClient{})
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "   ", PublicKey: testPublicKey})
		if !errors.Is(err, ErrNameRequired) {
			t.Fatalf("expected ErrNameRequired, got %v", err)
		}
	})

	t.Run("rejects empty public key", func(t *testing.T) {
		svc := newTestService(&fakeClient{})
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: "   "})
		if !errors.Is(err, ErrPublicKeyRequired) {
			t.Fatalf("expected ErrPublicKeyRequired, got %v", err)
		}
	})

	t.Run("rejects invalid public key format", func(t *testing.T) {
		svc := newTestService(&fakeClient{})
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: "not-a-key"})
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

		svc := newTestService(repo)
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
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

		svc := newTestService(repo)
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
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

		svc := newTestService(repo)
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
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

		svc := newTestService(repo)
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
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

		svc := newTestService(repo)
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
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

		svc := newTestService(repo)
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})

	t.Run("deletes created keypair when ownership save fails", func(t *testing.T) {
		saveErr := errors.New("db write failed")
		var deletedKeyPair string
		client := &fakeClient{
			getKeyPairFn: func(name string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusNotFound)
			},
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return &KeyPair{Name: name, PublicKey: publicKey}, nil
			},
			deleteKeyPairFn: func(name string) error {
				deletedKeyPair = name
				return nil
			},
		}
		repo := &noopKeyPairRepository{
			saveKeyPairFn: func(ctx context.Context, userID uuid.UUID, name string) error {
				return saveErr
			},
		}

		svc := newTestServiceWithRepo(client, repo)
		_, err := svc.CreateKeyPair(context.Background(), accessTestUserID, CreateKeyPairRequest{
			Name:      "team-default-key",
			PublicKey: testPublicKey,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if deletedKeyPair != "team-default-key" {
			t.Fatalf("expected cleanup delete for team-default-key, got %q", deletedKeyPair)
		}
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})
}

func TestServiceDeleteKeyPair(t *testing.T) {
	t.Run("returns nil when stale row delete fails after cloud delete", func(t *testing.T) {
		client := &fakeClient{
			deleteKeyPairFn: func(name string) error { return nil },
		}
		repo := &noopKeyPairRepository{
			isOwnerFn: func(ctx context.Context, userID uuid.UUID, name string) (bool, error) {
				return true, nil
			},
			deleteKeyPairFn: func(ctx context.Context, userID uuid.UUID, name string) error {
				return errors.New("db delete failed")
			},
		}

		svc := newTestServiceWithRepo(client, repo)
		if err := svc.DeleteKeyPair(context.Background(), accessTestUserID, "team-default-key"); err != nil {
			t.Fatalf("expected nil on stale row, got %v", err)
		}
	})
}
