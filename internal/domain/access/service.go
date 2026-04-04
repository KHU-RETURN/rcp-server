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
	ErrInvalidKeyPairRequest  = errors.New("invalid keypair request")
	ErrKeyPairAccessDenied    = errors.New("keypair access denied")
	ErrKeyPairOperationFailed = errors.New("failed to create keypair")
)

// keyPairClient는 OpenStack keypair API 접근 인터페이스입니다.
// 구현체는 client.go의 Client입니다.
type keyPairClient interface {
	GetKeyPair(name string) (*KeyPair, error)
	CreateKeyPair(name, publicKey string) (*KeyPair, error)
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
