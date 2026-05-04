package ssh

import (
	"context"
	"errors"
)

var (
	ErrInvalidNonce = errors.New("invalid nonce")
	ErrInvalidEmail = errors.New("invalid email")
)

// notifier is the package-private contract. *NotifyClient satisfies it.
type notifier interface {
	Notify(ctx context.Context, nonce, userEmail string) error
}

type Service struct {
	n notifier
}

func NewService(n notifier) *Service { return &Service{n: n} }

func (s *Service) HandleSSHCallback(ctx context.Context, nonce, userEmail string) error {
	if nonce == "" {
		return ErrInvalidNonce
	}
	if userEmail == "" {
		return ErrInvalidEmail
	}
	return s.n.Notify(ctx, nonce, userEmail)
}
