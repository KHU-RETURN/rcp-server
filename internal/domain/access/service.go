package access

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

var (
	ErrNameRequired              = errors.New("name is required")
	ErrPublicKeyRequired         = errors.New("public_key is required")
	ErrInvalidSSHKeyFormat       = errors.New("invalid SSH public key format")
	ErrKeyPairAlreadyExists      = errors.New("name already exists")
	ErrKeyPairNotFound           = errors.New("keypair not found")
	ErrInvalidKeyPairRequest     = errors.New("invalid keypair request")
	ErrKeyPairAccessDenied       = errors.New("keypair access denied")
	ErrKeyPairOperationFailed    = errors.New("failed to create keypair")
	ErrKeyPairDeleteFailed       = errors.New("failed to delete keypair")
	ErrConsoleInstanceIDRequired = errors.New("instance_id is required")
	ErrConsoleInstanceNotFound   = errors.New("instance not found")
	ErrConsoleInstanceNoIP       = errors.New("instance has no reachable IP")
	ErrConsoleOperationFailed    = errors.New("failed to create console session")
)

type keyPairClient interface {
	CreateKeyPair(name, publicKey string) (*KeyPair, error)
	DeleteKeyPair(name string) error
	GetInstance(id string) (*ConsoleInstance, error)
}

type keyPairRepo interface {
	SaveKeyPair(ctx context.Context, ownerID uuid.UUID, kp *KeyPair) error
	DeleteByName(ctx context.Context, ownerID uuid.UUID, name string) error
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]KeyPair, error)
	FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*KeyPair, error)
	FindConsoleTarget(ctx context.Context, ownerID uuid.UUID, openstackID string) (*ConsoleTarget, error)
}

type Service struct {
	client       keyPairClient
	repo         keyPairRepo
	consoleStore *consoleSessionStore
}

func NewService(client keyPairClient, repo keyPairRepo) *Service {
	return &Service{
		client:       client,
		repo:         repo,
		consoleStore: newConsoleSessionStore(2 * time.Minute),
	}
}

func (s *Service) CreateKeyPair(ctx context.Context, ownerID uuid.UUID, req CreateKeyPairRequest) (*KeyPairResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrNameRequired
	}

	publicKey := strings.TrimSpace(req.PublicKey)
	if publicKey == "" {
		return nil, ErrPublicKeyRequired
	}

	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey)); err != nil {
		return nil, ErrInvalidSSHKeyFormat
	}

	existing, err := s.repo.FindByName(ctx, ownerID, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyPairOperationFailed, err)
	}
	if existing != nil {
		return nil, ErrKeyPairAlreadyExists
	}

	kp, err := s.client.CreateKeyPair(name, publicKey)
	if err != nil {
		if isConflictError(err) {
			return nil, ErrKeyPairAlreadyExists
		}
		return nil, normalizeKeyPairError(err)
	}

	if err := s.repo.SaveKeyPair(ctx, ownerID, kp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyPairOperationFailed, err)
	}

	return &KeyPairResponse{
		Name:        kp.Name,
		Fingerprint: kp.Fingerprint,
		PublicKey:   kp.PublicKey,
	}, nil
}

func (s *Service) ListKeyPairs(ctx context.Context, ownerID uuid.UUID) ([]KeyPairResponse, error) {
	kps, err := s.repo.ListByOwner(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyPairOperationFailed, err)
	}

	result := make([]KeyPairResponse, len(kps))
	for i, kp := range kps {
		result[i] = KeyPairResponse(kp)
	}
	return result, nil
}

func (s *Service) GetKeyPair(ctx context.Context, ownerID uuid.UUID, name string) (*KeyPairResponse, error) {
	kp, err := s.repo.FindByName(ctx, ownerID, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyPairOperationFailed, err)
	}
	if kp == nil {
		return nil, ErrKeyPairNotFound
	}
	return &KeyPairResponse{
		Name:        kp.Name,
		Fingerprint: kp.Fingerprint,
		PublicKey:   kp.PublicKey,
	}, nil
}

func (s *Service) DeleteKeyPair(ctx context.Context, ownerID uuid.UUID, name string) error {
	kp, err := s.repo.FindByName(ctx, ownerID, name)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrKeyPairOperationFailed, err)
	}
	if kp == nil {
		return ErrKeyPairNotFound
	}

	if err := s.client.DeleteKeyPair(name); err != nil {
		if isNotFoundError(err) {
			return ErrKeyPairNotFound
		}
		return fmt.Errorf("%w: %v", ErrKeyPairDeleteFailed, err)
	}

	if err := s.repo.DeleteByName(ctx, ownerID, name); err != nil {
		return fmt.Errorf("%w: %v", ErrKeyPairDeleteFailed, err)
	}
	return nil
}

func (s *Service) CreateConsoleSession(ctx context.Context, ownerID uuid.UUID, instanceID string, req CreateConsoleSessionRequest, basePath string) (*CreateConsoleSessionResponse, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, ErrConsoleInstanceIDRequired
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = "ubuntu"
	}

	target, err := s.repo.FindConsoleTarget(ctx, ownerID, instanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConsoleOperationFailed, err)
	}
	if target == nil {
		return nil, ErrConsoleInstanceNotFound
	}

	instance, err := s.client.GetInstance(instanceID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, ErrConsoleInstanceNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrConsoleOperationFailed, err)
	}

	host := firstNonEmpty(instance.FixedIP, instance.FloatingIP)
	if host == "" {
		return nil, ErrConsoleInstanceNoIP
	}

	signer, authorizedKey, err := generateEphemeralSSHKey()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConsoleOperationFailed, err)
	}

	session, err := s.consoleStore.Create(consoleSession{
		OwnerID:       ownerID,
		InstanceID:    instanceID,
		Host:          host,
		Username:      username,
		Signer:        signer,
		AuthorizedKey: authorizedKey,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConsoleOperationFailed, err)
	}

	return &CreateConsoleSessionResponse{
		URL:       strings.TrimRight(basePath, "/") + routeAccessPrefix + pathConsoleWebSocket + "?token=" + session.Token,
		ExpiresAt: session.ExpiresAt,
	}, nil
}

func (s *Service) TakeConsoleSession(token string) (*consoleSession, bool) {
	return s.consoleStore.Take(token)
}

func (s *Service) AuthorizedKeys(instanceID, username string) string {
	return s.consoleStore.AuthorizedKeys(strings.TrimSpace(instanceID), strings.TrimSpace(username))
}

func (s *Service) DeleteAuthorizedKey(instanceID, username, key string) {
	s.consoleStore.DeleteAuthorizedKey(strings.TrimSpace(instanceID), strings.TrimSpace(username), key)
}

func (s *Service) AddEphemeralAuthorizedKey(req EphemeralAuthorizedKeyRequest) error {
	instanceID := strings.TrimSpace(req.InstanceID)
	if instanceID == "" {
		return ErrConsoleInstanceIDRequired
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = "ubuntu"
	}
	key := strings.TrimSpace(req.AuthorizedKey)
	if key == "" {
		return ErrPublicKeyRequired
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key)); err != nil {
		return ErrInvalidSSHKeyFormat
	}
	s.consoleStore.AddAuthorizedKey(instanceID, username, key)
	return nil
}

func (s *Service) DeleteEphemeralAuthorizedKey(req EphemeralAuthorizedKeyRequest) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = "ubuntu"
	}
	s.consoleStore.DeleteAuthorizedKey(strings.TrimSpace(req.InstanceID), username, strings.TrimSpace(req.AuthorizedKey))
}

func isNotFoundError(err error) bool {
	return hasStatusCode(err, http.StatusNotFound)
}

func isConflictError(err error) bool {
	return hasStatusCode(err, http.StatusConflict)
}

func normalizeKeyPairError(err error) error {
	switch {
	case hasStatusCode(err, http.StatusBadRequest):
		return fmt.Errorf("%w: %v", ErrInvalidKeyPairRequest, err)
	case hasStatusCode(err, http.StatusForbidden):
		return fmt.Errorf("%w: %v", ErrKeyPairAccessDenied, err)
	default:
		return fmt.Errorf("%w: %v", ErrKeyPairOperationFailed, err)
	}
}

func hasStatusCode(err error, expected int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == expected
}

func generateEphemeralSSHKey() (ssh.Signer, string, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, "", err
	}
	return signer, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
