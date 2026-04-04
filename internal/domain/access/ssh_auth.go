package access

import (
	"bytes"
	"context"
	"fmt"
	"os"

	gossh "golang.org/x/crypto/ssh"
)

// UserVerifier checks whether an email belongs to a registered RCP user.
// This interface decouples the SSH relay from the auth domain's concrete types.
type UserVerifier interface {
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

// buildSSHServerConfig constructs the gossh.ServerConfig that validates
// Cloudflare short-lived SSH certificates.
//
// Auth flow:
//  1. Client presents a cert signed by CF CA.
//  2. IsUserAuthority verifies the cert's CA against the known CF CA public key.
//  3. The cert's first ValidPrincipal (user's email) is checked against the RCP DB.
//  4. On success, the email is stored in Permissions.Extensions["email"]
//     so the connection handler can retrieve it later.
func buildSSHServerConfig(hostKey gossh.Signer, cfCAKey gossh.PublicKey, verifier UserVerifier) *gossh.ServerConfig {
	certChecker := &gossh.CertChecker{
		IsUserAuthority: func(auth gossh.PublicKey) bool {
			return bytes.Equal(auth.Marshal(), cfCAKey.Marshal())
		},
		IsRevoked: func(_ *gossh.Certificate) bool {
			return false
		},
	}

	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			cert, ok := key.(*gossh.Certificate)
			if !ok {
				return nil, fmt.Errorf("auth: not a certificate")
			}

			// Verify cert signature against CF CA
			if err := certChecker.CheckCert(conn.User(), cert); err != nil {
				return nil, fmt.Errorf("auth: cert check failed: %w", err)
			}

			// Extract email from certificate principals
			if len(cert.ValidPrincipals) == 0 {
				return nil, fmt.Errorf("auth: cert has no principals")
			}
			email := cert.ValidPrincipals[0]

			// Verify the email is a registered RCP user
			exists, err := verifier.ExistsByEmail(context.Background(), email)
			if err != nil {
				return nil, fmt.Errorf("auth: user lookup failed: %w", err)
			}
			if !exists {
				return nil, fmt.Errorf("auth: user %q not registered", email)
			}

			return &gossh.Permissions{
				Extensions: map[string]string{
					permissionEmailKey: email,
				},
			}, nil
		},
	}
	cfg.AddHostKey(hostKey)
	return cfg
}

// loadSSHHostKey reads and parses the SSH host private key from the given path.
func loadSSHHostKey(path string) (gossh.Signer, error) {
	return readSSHKey(path, gossh.ParsePrivateKey)
}

// loadCFCAKey reads and parses the Cloudflare CA public key from the given path.
func loadCFCAKey(path string) (gossh.PublicKey, error) {
	return readSSHKey(path, func(b []byte) (gossh.PublicKey, error) {
		pub, _, _, _, err := gossh.ParseAuthorizedKey(b)
		return pub, err
	})
}

// loadSSHServiceKey reads and parses the RCP service private key used to authenticate to VMs.
func loadSSHServiceKey(path string) (gossh.Signer, error) {
	return readSSHKey(path, gossh.ParsePrivateKey)
}

// readSSHKey reads a key file and parses it with the provided parser.
// Path comes from app config at startup, not from user input.
func readSSHKey[T any](path string, parse func([]byte) (T, error)) (T, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from trusted config
	if err != nil {
		var zero T
		return zero, fmt.Errorf("read key %s: %w", path, err)
	}
	result, err := parse(b)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("parse key %s: %w", path, err)
	}
	return result, nil
}
