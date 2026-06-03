package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

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
		prompt := formatAuthPrompt(cfg.AuthURLBase, p.Nonce, p.Code, cfg.NonceTTL)
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

func formatAuthPrompt(authURLBase, nonce, code string, timeout time.Duration) string {
	authURL := fmt.Sprintf("%s/ssh-auth?s=%s", authURLBase, nonce)
	return fmt.Sprintf(
		"%sRCP SSH browser authentication required.\r\n\r\n"+
			"1. Open this URL in your browser:\r\n   %s\r\n\r\n"+
			"2. Enter this 6-digit code on the auth page:\r\n%s\r\n\r\n"+
			"If your terminal supports clipboard integration, the code was copied automatically.\r\n"+
			"Waiting for browser authentication. Timeout: %s\r\n",
		terminalClipboardCopy(code),
		terminalHyperlink(authURL, authURL),
		code,
		timeout,
	)
}

func terminalHyperlink(url, label string) string {
	return fmt.Sprintf("\x1b]8;;%s\a%s\x1b]8;;\a", url, label)
}

func terminalClipboardCopy(text string) string {
	return fmt.Sprintf("\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
}
