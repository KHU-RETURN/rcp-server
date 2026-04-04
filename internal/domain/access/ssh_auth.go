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
					"email": email,
				},
			}, nil
		},
	}
	cfg.AddHostKey(hostKey)
	return cfg
}

// loadSSHHostKey reads and parses the SSH host private key from the given path.
func loadSSHHostKey(path string) (gossh.Signer, error) {
	keyBytes, err := os.ReadFile(path) //nolint:gosec // path comes from trusted config
	if err != nil {
		return nil, fmt.Errorf("loadSSHHostKey: read %s: %w", path, err)
	}
	signer, err := gossh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("loadSSHHostKey: parse %s: %w", path, err)
	}
	return signer, nil
}

// loadCFCAKey reads and parses the Cloudflare CA public key from the given path.
func loadCFCAKey(path string) (gossh.PublicKey, error) {
	keyBytes, err := os.ReadFile(path) //nolint:gosec // path comes from trusted config
	if err != nil {
		return nil, fmt.Errorf("loadCFCAKey: read %s: %w", path, err)
	}
	pubKey, _, _, _, err := gossh.ParseAuthorizedKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("loadCFCAKey: parse %s: %w", path, err)
	}
	return pubKey, nil
}

// loadSSHServiceKey reads and parses the RCP service private key used to authenticate to VMs.
func loadSSHServiceKey(path string) (gossh.Signer, error) {
	keyBytes, err := os.ReadFile(path) //nolint:gosec // path comes from trusted config
	if err != nil {
		return nil, fmt.Errorf("loadSSHServiceKey: read %s: %w", path, err)
	}
	signer, err := gossh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("loadSSHServiceKey: parse %s: %w", path, err)
	}
	return signer, nil
}
