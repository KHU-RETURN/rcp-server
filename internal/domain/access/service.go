package access

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	ListKeyPairs() ([]KeyPair, error)
	GetKeyPair(name string) (*KeyPair, error)
	CreateKeyPair(name, publicKey string) (*KeyPair, error)
	DeleteKeyPair(name string) error
}

type Service struct {
	client keyPairClient
}

func NewService(client keyPairClient) *Service {
	return &Service{client: client}
}

func (s *Service) CreateKeyPair(req CreateKeyPairRequest) (*KeyPairResponse, error) {
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

	_, err := s.client.GetKeyPair(name)
	switch {
	case err == nil:
		return nil, ErrKeyPairAlreadyExists
	case isNotFoundError(err):
		// 존재하지 않음 → 생성 진행
	default:
		return nil, normalizeKeyPairError(err)
	}

	keyPair, err := s.client.CreateKeyPair(name, publicKey)
	if err != nil {
		if isConflictError(err) {
			return nil, ErrKeyPairAlreadyExists
		}
		return nil, normalizeKeyPairError(err)
	}

	return &KeyPairResponse{
		Name:        keyPair.Name,
		Fingerprint: keyPair.Fingerprint,
		PublicKey:   keyPair.PublicKey,
	}, nil
}

func (s *Service) ListKeyPairs() ([]KeyPairResponse, error) {
	kps, err := s.client.ListKeyPairs()
	if err != nil {
		return nil, normalizeKeyPairError(err)
	}

	result := make([]KeyPairResponse, len(kps))
	for i, kp := range kps {
		result[i] = KeyPairResponse(kp)
	}
	return result, nil
}

func (s *Service) GetKeyPair(name string) (*KeyPairResponse, error) {
	kp, err := s.client.GetKeyPair(name)
	if err != nil {
		if isNotFoundError(err) {
			return nil, ErrKeyPairNotFound
		}
		return nil, normalizeKeyPairError(err)
	}

	return &KeyPairResponse{
		Name:        kp.Name,
		Fingerprint: kp.Fingerprint,
		PublicKey:   kp.PublicKey,
	}, nil
}

func (s *Service) DeleteKeyPair(name string) error {
	if err := s.client.DeleteKeyPair(name); err != nil {
		if isNotFoundError(err) {
			return ErrKeyPairNotFound
		}
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
