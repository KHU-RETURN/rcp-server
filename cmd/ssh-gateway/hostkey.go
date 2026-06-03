package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// LoadOrCreateHostKey returns an ssh.Signer for the gateway's host key. If
// the file is missing, a new ed25519 key is generated and written with 0o600.
func LoadOrCreateHostKey(path string) (ssh.Signer, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is operator-controlled, not user input
	if err == nil {
		s, err := ssh.ParsePrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("parse host key %s: %w", path, err)
		}
		return s, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read host key %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	pemBytes, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, fmt.Errorf("marshal host key: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBytes), 0o600); err != nil {
		return nil, fmt.Errorf("write host key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	return signer, nil
}

func loadInnerHostKeyCallback(path string) (ssh.HostKeyCallback, error) {
	return knownhosts.New(path)
}

func reloadingInnerHostKeyCallback(path string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		cb, err := loadInnerHostKeyCallback(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return trustInnerHostKeyOnFirstUse(path, hostname, key)
			}
			return fmt.Errorf("inner host key trust unavailable at %s: %w", path, err)
		}
		if err := cb(hostname, remote, key); err != nil {
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
				return trustInnerHostKeyOnFirstUse(path, hostname, key)
			}
			return err
		}
		return nil
	}
}

func trustInnerHostKeyOnFirstUse(path, hostname string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("prepare inner host key trust store %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640) //nolint:gosec // operator-controlled trust store path
	if err != nil {
		return fmt.Errorf("open inner host key trust store %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("append inner host key trust store %s: %w", path, err)
	}
	return nil
}
