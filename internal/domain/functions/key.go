package functions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
)

const keyPrefix = "rcpf_"
const defaultKeyDays = 30
const maxKeyDays = 90

var ErrInvalidKey = errors.New("invalid or expired function key")
var ErrInvalidExpiry = errors.New("key expiry must be 1 to 90 days")

type IssuedKey struct {
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) IssueKey(ctx context.Context, owner, id uuid.UUID, days int) (*IssuedKey, error) {
	if days == 0 {
		days = defaultKeyDays
	}
	if days < 1 || days > maxKeyDays {
		return nil, ErrInvalidExpiry
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	key := keyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(key))
	created := s.now().UTC()
	expires := created.AddDate(0, 0, days)
	if _, err := s.repo.SetKey(ctx, owner, id, hash[:], created, expires); err != nil {
		return nil, err
	}
	return &IssuedKey{Key: key, ExpiresAt: expires}, nil
}

func (s *Service) RevokeKey(ctx context.Context, owner, id uuid.UUID) error {
	_, err := s.repo.DeleteKey(ctx, owner, id)
	return err
}

func (s *Service) checkKey(fn *Function, key string) error {
	if len(fn.KeyHash) != sha256.Size || fn.KeyExpiresAt == nil || !s.now().Before(*fn.KeyExpiresAt) {
		return ErrInvalidKey
	}
	hash := sha256.Sum256([]byte(key))
	if subtle.ConstantTimeCompare(hash[:], fn.KeyHash) != 1 || len(key) != len(keyPrefix)+43 {
		return ErrInvalidKey
	}
	return nil
}
