package access

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

var (
	ErrNameRequired           = errors.New("name is required")
	ErrPublicKeyRequired      = errors.New("public_key is required")
	ErrInvalidSSHKeyFormat    = errors.New("invalid SSH public key format")
	ErrKeyPairAlreadyExists   = errors.New("name already exists")
	ErrKeyPairNotFound        = errors.New("keypair not found")
	ErrInvalidKeyPairRequest  = errors.New("invalid keypair request")
	ErrKeyPairAccessDenied    = errors.New("keypair access denied")
	ErrKeyPairOperationFailed = errors.New("failed to create keypair")
	ErrKeyPairDeleteFailed    = errors.New("failed to delete keypair")
)

// keyPairClient는 OpenStack keypair API 접근 인터페이스입니다.
// 구현체는 client.go의 Client입니다.
type keyPairClient interface {
	CreateKeyPair(name, publicKey string) (*KeyPair, error)
	DeleteKeyPair(name string) error
}

type keyPairRepo interface {
	SaveKeyPair(ctx context.Context, ownerID uuid.UUID, kp *KeyPair) error
	DeleteByName(ctx context.Context, ownerID uuid.UUID, name string) error
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]KeyPair, error)
	FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*KeyPair, error)
}

type Service struct {
	client keyPairClient
	repo   keyPairRepo
}

func NewService(client keyPairClient, repo keyPairRepo) *Service {
	return &Service{client: client, repo: repo}
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
