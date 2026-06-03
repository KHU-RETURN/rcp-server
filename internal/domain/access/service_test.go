package access

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMz7v3R7iK4WbG2ZrM8Z8vV7n6lYx4l6Wwq8m7M+v7gL test@example"

var testOwnerID = uuid.New()

type fakeClient struct {
	createKeyPairFn func(name, publicKey string) (*KeyPair, error)
	deleteKeyPairFn func(name string) error
	getInstanceFn   func(id string) (*ConsoleInstance, error)
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

func (f *fakeClient) GetInstance(id string) (*ConsoleInstance, error) {
	if f.getInstanceFn != nil {
		return f.getInstanceFn(id)
	}
	return nil, nil
}

type fakeRepo struct {
	findByNameFn        func(ctx context.Context, ownerID uuid.UUID, name string) (*KeyPair, error)
	saveKeyPairFn       func(ctx context.Context, ownerID uuid.UUID, kp *KeyPair) error
	deleteByNameFn      func(ctx context.Context, ownerID uuid.UUID, name string) error
	listByOwnerFn       func(ctx context.Context, ownerID uuid.UUID) ([]KeyPair, error)
	findConsoleTargetFn func(ctx context.Context, ownerID uuid.UUID, openstackID string) (*ConsoleTarget, error)
}

func (r *fakeRepo) FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*KeyPair, error) {
	if r.findByNameFn != nil {
		return r.findByNameFn(ctx, ownerID, name)
	}
	return nil, nil
}

func (r *fakeRepo) SaveKeyPair(ctx context.Context, ownerID uuid.UUID, kp *KeyPair) error {
	if r.saveKeyPairFn != nil {
		return r.saveKeyPairFn(ctx, ownerID, kp)
	}
	return nil
}

func (r *fakeRepo) DeleteByName(ctx context.Context, ownerID uuid.UUID, name string) error {
	if r.deleteByNameFn != nil {
		return r.deleteByNameFn(ctx, ownerID, name)
	}
	return nil
}

func (r *fakeRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]KeyPair, error) {
	if r.listByOwnerFn != nil {
		return r.listByOwnerFn(ctx, ownerID)
	}
	return nil, nil
}

func (r *fakeRepo) FindConsoleTarget(ctx context.Context, ownerID uuid.UUID, openstackID string) (*ConsoleTarget, error) {
	if r.findConsoleTargetFn != nil {
		return r.findConsoleTargetFn(ctx, ownerID, openstackID)
	}
	return nil, nil
}

func newStatusErr(code int) *StatusError {
	return &StatusError{Code: code, Err: errors.New(http.StatusText(code))}
}

func TestServiceCreateKeyPair(t *testing.T) {
	ctx := context.Background()

	t.Run("creates keypair when input is valid", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return &KeyPair{Name: name, Fingerprint: "fingerprint", PublicKey: publicKey}, nil
			},
		}

		svc := NewService(osClient, &fakeRepo{})
		res, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{
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
		svc := NewService(&fakeClient{}, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "   ", PublicKey: testPublicKey})
		if !errors.Is(err, ErrNameRequired) {
			t.Fatalf("expected ErrNameRequired, got %v", err)
		}
	})

	t.Run("rejects empty public key", func(t *testing.T) {
		svc := NewService(&fakeClient{}, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: "   "})
		if !errors.Is(err, ErrPublicKeyRequired) {
			t.Fatalf("expected ErrPublicKeyRequired, got %v", err)
		}
	})

	t.Run("rejects invalid public key format", func(t *testing.T) {
		svc := NewService(&fakeClient{}, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: "not-a-key"})
		if !errors.Is(err, ErrInvalidSSHKeyFormat) {
			t.Fatalf("expected ErrInvalidSSHKeyFormat, got %v", err)
		}
	})

	t.Run("returns conflict when keypair already exists in DB", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				t.Fatal("create should not be called when keypair exists")
				return nil, nil
			},
		}
		repo := &fakeRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, name string) (*KeyPair, error) {
				return &KeyPair{Name: name}, nil
			},
		}

		svc := NewService(osClient, repo)
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairAlreadyExists) {
			t.Fatalf("expected ErrKeyPairAlreadyExists, got %v", err)
		}
	})

	t.Run("normalizes create conflict as conflict error", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusConflict)
			},
		}

		svc := NewService(osClient, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairAlreadyExists) {
			t.Fatalf("expected ErrKeyPairAlreadyExists, got %v", err)
		}
	})

	t.Run("returns operation failed when repo FindByName errors", func(t *testing.T) {
		repo := &fakeRepo{
			findByNameFn: func(_ context.Context, _ uuid.UUID, _ string) (*KeyPair, error) {
				return nil, errors.New("db error")
			},
		}

		svc := NewService(&fakeClient{}, repo)
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})

	t.Run("normalizes upstream forbidden on create", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusForbidden)
			},
		}

		svc := NewService(osClient, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairAccessDenied) {
			t.Fatalf("expected ErrKeyPairAccessDenied, got %v", err)
		}
	})

	t.Run("normalizes upstream bad request on create", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusBadRequest)
			},
		}

		svc := NewService(osClient, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrInvalidKeyPairRequest) {
			t.Fatalf("expected ErrInvalidKeyPairRequest, got %v", err)
		}
	})

	t.Run("normalizes upstream server error on create", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, newStatusErr(http.StatusInternalServerError)
			},
		}

		svc := NewService(osClient, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})

	t.Run("normalizes generic openstack errors as internal operation error", func(t *testing.T) {
		osClient := &fakeClient{
			createKeyPairFn: func(name, publicKey string) (*KeyPair, error) {
				return nil, errors.New("unexpected error")
			},
		}

		svc := NewService(osClient, &fakeRepo{})
		_, err := svc.CreateKeyPair(ctx, testOwnerID, CreateKeyPairRequest{Name: "key", PublicKey: testPublicKey})
		if !errors.Is(err, ErrKeyPairOperationFailed) {
			t.Fatalf("expected ErrKeyPairOperationFailed, got %v", err)
		}
	})
}

func TestServiceCreateConsoleSession(t *testing.T) {
	ctx := context.Background()

	t.Run("creates one-time console token for owned instance", func(t *testing.T) {
		repo := &fakeRepo{
			findConsoleTargetFn: func(_ context.Context, ownerID uuid.UUID, openstackID string) (*ConsoleTarget, error) {
				if ownerID != testOwnerID {
					t.Fatalf("unexpected ownerID: %s", ownerID)
				}
				if openstackID != "server-1" {
					t.Fatalf("unexpected openstackID: %q", openstackID)
				}
				return &ConsoleTarget{
					Instance: ConsoleInstance{ID: openstackID, Name: "server-1"},
				}, nil
			},
		}
		client := &fakeClient{
			getInstanceFn: func(id string) (*ConsoleInstance, error) {
				if id != "server-1" {
					t.Fatalf("unexpected instance id: %q", id)
				}
				return &ConsoleInstance{ID: id, FixedIP: "10.0.0.8"}, nil
			},
		}

		svc := NewService(client, repo)
		res, err := svc.CreateConsoleSession(ctx, testOwnerID, " server-1 ", CreateConsoleSessionRequest{
			Username: " ubuntu ",
		}, "ws://example.test/api/v1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		token := consoleTokenFromURL(t, res.URL)
		if token == "" || !strings.Contains(res.URL, "/access/console/ws?token="+token) {
			t.Fatalf("unexpected console response: %+v", res)
		}
		authorizedKeys := svc.AuthorizedKeys("server-1", "ubuntu")
		if !strings.HasPrefix(authorizedKeys, "ssh-ed25519 ") {
			t.Fatalf("expected ephemeral authorized key, got %q", authorizedKeys)
		}

		session, ok := svc.TakeConsoleSession(token)
		if !ok {
			t.Fatal("expected token to be accepted")
		}
		if session.Host != "10.0.0.8" || session.Username != "ubuntu" || session.InstanceID != "server-1" {
			t.Fatalf("unexpected stored session: %+v", session)
		}
		svc.DeleteAuthorizedKey(session.InstanceID, session.Username, session.AuthorizedKey)
		if keys := svc.AuthorizedKeys("server-1", "ubuntu"); keys != "" {
			t.Fatalf("expected authorized key to be deleted, got %q", keys)
		}
		if _, ok := svc.TakeConsoleSession(token); ok {
			t.Fatal("expected token to be single-use")
		}
	})

	t.Run("rejects unowned instance", func(t *testing.T) {
		svc := NewService(&fakeClient{}, &fakeRepo{
			findConsoleTargetFn: func(context.Context, uuid.UUID, string) (*ConsoleTarget, error) {
				return nil, nil
			},
		})

		_, err := svc.CreateConsoleSession(ctx, testOwnerID, "server-1", CreateConsoleSessionRequest{
			Username: "ubuntu",
		}, "ws://example.test/api/v1")
		if !errors.Is(err, ErrConsoleInstanceNotFound) {
			t.Fatalf("expected ErrConsoleInstanceNotFound, got %v", err)
		}
	})
}

func TestServiceEphemeralAuthorizedKey(t *testing.T) {
	svc := NewService(&fakeClient{}, &fakeRepo{})

	req := EphemeralAuthorizedKeyRequest{
		InstanceID:    " server-1 ",
		Username:      " ubuntu ",
		AuthorizedKey: " " + testPublicKey + " ",
	}
	if err := svc.AddEphemeralAuthorizedKey(req); err != nil {
		t.Fatalf("add ephemeral key: %v", err)
	}

	keys := svc.AuthorizedKeys("server-1", "ubuntu")
	if keys != testPublicKey+"\n" {
		t.Fatalf("authorized keys = %q", keys)
	}

	svc.DeleteEphemeralAuthorizedKey(req)
	if keys := svc.AuthorizedKeys("server-1", "ubuntu"); keys != "" {
		t.Fatalf("expected key deleted, got %q", keys)
	}
}

func consoleTokenFromURL(t *testing.T, rawURL string) string {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("invalid console URL %q: %v", rawURL, err)
	}
	return u.Query().Get("token")
}
