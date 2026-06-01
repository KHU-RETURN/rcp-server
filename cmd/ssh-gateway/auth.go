package main

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
)

const (
	permEmailKey = "rcp-user-email"
	permNonceKey = "rcp-nonce"
)

// kbdInteractiveAuthenticator returns an ssh.ServerConfig.KeyboardInteractiveCallback
// that drives the OAuth-URL keyboard-interactive flow.
func kbdInteractiveAuthenticator(cfg *Config, store *sessionStore) func(ssh.ConnMetadata, ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
	return func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		p, err := store.New()
		if err != nil {
			if errors.Is(err, ErrTooManyPending) {
				return nil, errors.New("too many pending auth sessions")
			}
			return nil, fmt.Errorf("alloc nonce: %w", err)
		}
		prompt := fmt.Sprintf(
			"Open: %s/ssh-auth?s=%s\r\nCode: %s\r\n(waiting for browser auth, %s timeout)\r\n",
			cfg.AuthURLBase, p.Nonce, p.Code, cfg.NonceTTL,
		)
		// Send the URL as an instruction with no answers requested. OpenSSH
		// renders "instruction" before any prompts.
		if _, err := challenge("", prompt, nil, nil); err != nil {
			return nil, err
		}
		email, err := p.Wait(cfg.NonceTTL)
		if err != nil {
			if errors.Is(err, ErrNonceExpired) {
				return nil, errors.New("auth timeout")
			}
			return nil, err
		}
		return &ssh.Permissions{
			Extensions: map[string]string{
				permEmailKey: email,
				permNonceKey: p.Nonce,
			},
		}, nil
	}
}
