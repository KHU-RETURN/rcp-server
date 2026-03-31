package access

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
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

type keyPairRepository interface {
	GetComputeClient() (*gophercloud.ServiceClient, error)
	GetKeyPair(client *gophercloud.ServiceClient, name string) (*keypairs.KeyPair, error)
	CreateKeyPair(client *gophercloud.ServiceClient, name, publicKey string) (*keypairs.KeyPair, error)
}

type Service struct {
	Repo keyPairRepository
}

func NewService(repo keyPairRepository) *Service {
	return &Service{Repo: repo}
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

	client, err := s.Repo.GetComputeClient()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyPairOperationFailed, err)
	}

	_, err = s.Repo.GetKeyPair(client, name)
	switch {
	case err == nil:
		return nil, ErrKeyPairAlreadyExists
	case isNotFoundError(err):
	default:
		return nil, normalizeKeyPairError(err)
	}

	keyPair, err := s.Repo.CreateKeyPair(client, name, publicKey)
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
	var statusErr gophercloud.ErrDefault404
	if errors.As(err, &statusErr) {
		return true
	}

	var codeErr gophercloud.StatusCodeError
	return errors.As(err, &codeErr) && codeErr.GetStatusCode() == 404
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
	switch expected {
	case http.StatusBadRequest:
		var statusErr gophercloud.ErrDefault400
		if errors.As(err, &statusErr) {
			return true
		}
	case http.StatusForbidden:
		var statusErr gophercloud.ErrDefault403
		if errors.As(err, &statusErr) {
			return true
		}
	case http.StatusNotFound:
		var statusErr gophercloud.ErrDefault404
		if errors.As(err, &statusErr) {
			return true
		}
	case http.StatusConflict:
		var statusErr gophercloud.ErrDefault409
		if errors.As(err, &statusErr) {
			return true
		}
	}

	var codeErr gophercloud.StatusCodeError
	return errors.As(err, &codeErr) && codeErr.GetStatusCode() == expected
}
