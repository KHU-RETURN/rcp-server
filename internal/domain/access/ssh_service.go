package access

import (
	"context"
	"errors"
)

var (
	ErrInvalidNonce   = errors.New("invalid nonce")
	ErrInvalidSSHCode = errors.New("invalid ssh code")
	ErrInvalidEmail   = errors.New("invalid email")
)

// sshNotifier is the package-private contract. *NotifyClient satisfies it.
type sshNotifier interface {
	Notify(ctx context.Context, nonce, code, userEmail string) error
}

type SSHService struct {
	n sshNotifier
}

func NewSSHService(n sshNotifier) *SSHService { return &SSHService{n: n} }

func (s *SSHService) HandleSSHCallback(ctx context.Context, nonce, code, userEmail string) error {
	if nonce == "" {
		return ErrInvalidNonce
	}
	if code == "" {
		return ErrInvalidSSHCode
	}
	if userEmail == "" {
		return ErrInvalidEmail
	}
	return s.n.Notify(ctx, nonce, code, userEmail)
}
