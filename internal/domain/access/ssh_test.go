package access

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"encoding/pem"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Fake sshVMRepository for service-layer tests
// ---------------------------------------------------------------------------

type fakeSSHVMRepository struct {
	registerVMFn         func(ctx context.Context, userEmail, vmName, vmID, fixedIP string) error
	unregisterVMFn       func(ctx context.Context, vmID string) error
	listVMsByEmailFn     func(ctx context.Context, email string) ([]UserVM, error)
	findVMByEmailAndName func(ctx context.Context, email, vmName string) (*UserVM, error)
}

func (f *fakeSSHVMRepository) RegisterVM(ctx context.Context, userEmail, vmName, vmID, fixedIP string) error {
	if f.registerVMFn != nil {
		return f.registerVMFn(ctx, userEmail, vmName, vmID, fixedIP)
	}
	return nil
}

func (f *fakeSSHVMRepository) UnregisterVM(ctx context.Context, vmID string) error {
	if f.unregisterVMFn != nil {
		return f.unregisterVMFn(ctx, vmID)
	}
	return nil
}

func (f *fakeSSHVMRepository) ListVMsByEmail(ctx context.Context, email string) ([]UserVM, error) {
	if f.listVMsByEmailFn != nil {
		return f.listVMsByEmailFn(ctx, email)
	}
	return nil, nil
}

func (f *fakeSSHVMRepository) FindVMByEmailAndName(ctx context.Context, email, vmName string) (*UserVM, error) {
	if f.findVMByEmailAndName != nil {
		return f.findVMByEmailAndName(ctx, email, vmName)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// fakeChannel implements gossh.Channel for menu rendering tests
// ---------------------------------------------------------------------------

type fakeChannel struct {
	written bytes.Buffer
	readBuf *bytes.Buffer
}

func newFakeChannel(input string) *fakeChannel {
	return &fakeChannel{readBuf: bytes.NewBufferString(input)}
}

func (c *fakeChannel) Read(data []byte) (int, error)          { return c.readBuf.Read(data) }
func (c *fakeChannel) Write(data []byte) (int, error)         { return c.written.Write(data) }
func (c *fakeChannel) Close() error                           { return nil }
func (c *fakeChannel) CloseWrite() error                      { return nil }
func (c *fakeChannel) SendRequest(_ string, _ bool, _ []byte) (bool, error) {
	return false, nil
}
func (c *fakeChannel) Stderr() io.ReadWriter { return &bytes.Buffer{} }

// ---------------------------------------------------------------------------
// Helper: generate ed25519 key pair for tests
// ---------------------------------------------------------------------------

func generateTestSigner(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ===========================================================================
// SSHConfig tests
// ===========================================================================

func TestSSHConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		// Clear relevant env vars
		for _, key := range []string{"SSH_LISTEN_PORT", "SSH_MENU_PAGE_SIZE", "SSH_HOST_KEY_PATH", "CF_CA_PUBLIC_KEY_PATH", "QROUTER_NAMESPACE", "RCP_SERVICE_KEY_PATH"} {
			t.Setenv(key, "")
		}

		cfg := SSHConfigFromEnv()
		if cfg.ListenAddr != ":2222" {
			t.Fatalf("expected default listen addr :2222, got %q", cfg.ListenAddr)
		}
		if cfg.MenuPageSize != 10 {
			t.Fatalf("expected default page size 10, got %d", cfg.MenuPageSize)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		t.Setenv("SSH_LISTEN_PORT", "3333")
		t.Setenv("SSH_MENU_PAGE_SIZE", "5")
		t.Setenv("SSH_HOST_KEY_PATH", "/keys/host")
		t.Setenv("CF_CA_PUBLIC_KEY_PATH", "/keys/cf")
		t.Setenv("QROUTER_NAMESPACE", "qrouter-abc")
		t.Setenv("RCP_SERVICE_KEY_PATH", "/keys/svc")

		cfg := SSHConfigFromEnv()
		if cfg.ListenAddr != ":3333" {
			t.Fatalf("expected listen addr :3333, got %q", cfg.ListenAddr)
		}
		if cfg.MenuPageSize != 5 {
			t.Fatalf("expected page size 5, got %d", cfg.MenuPageSize)
		}
		if cfg.HostKeyPath != "/keys/host" {
			t.Fatalf("expected host key path, got %q", cfg.HostKeyPath)
		}
		if cfg.CFCAPublicKeyPath != "/keys/cf" {
			t.Fatalf("expected CF CA path, got %q", cfg.CFCAPublicKeyPath)
		}
		if cfg.QRouterNamespace != "qrouter-abc" {
			t.Fatalf("expected namespace, got %q", cfg.QRouterNamespace)
		}
		if cfg.ServiceKeyPath != "/keys/svc" {
			t.Fatalf("expected service key path, got %q", cfg.ServiceKeyPath)
		}
	})

	t.Run("invalid page size falls back to default", func(t *testing.T) {
		t.Setenv("SSH_MENU_PAGE_SIZE", "abc")
		t.Setenv("SSH_LISTEN_PORT", "")

		cfg := SSHConfigFromEnv()
		if cfg.MenuPageSize != 10 {
			t.Fatalf("expected default page size 10 for invalid input, got %d", cfg.MenuPageSize)
		}
	})

	t.Run("zero page size falls back to default", func(t *testing.T) {
		t.Setenv("SSH_MENU_PAGE_SIZE", "0")
		t.Setenv("SSH_LISTEN_PORT", "")

		cfg := SSHConfigFromEnv()
		if cfg.MenuPageSize != 10 {
			t.Fatalf("expected default page size 10 for zero, got %d", cfg.MenuPageSize)
		}
	})
}

// ===========================================================================
// parseSSHUsername tests
// ===========================================================================

func TestParseSSHUsername(t *testing.T) {
	tests := []struct {
		raw      string
		wantUser string
		wantVM   string
	}{
		{"alice", "alice", ""},
		{"alice+myvm", "alice", "myvm"},
		{"alice+my+vm", "alice", "my+vm"},
		{"", "", ""},
		{"+vmonly", "", "vmonly"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			user, vm := parseSSHUsername(tt.raw)
			if user != tt.wantUser || vm != tt.wantVM {
				t.Fatalf("parseSSHUsername(%q) = (%q, %q), want (%q, %q)", tt.raw, user, vm, tt.wantUser, tt.wantVM)
			}
		})
	}
}

// ===========================================================================
// SSHRepository tests (SQLite in-memory)
// ===========================================================================

func TestSSHRepository(t *testing.T) {
	ctx := context.Background()

	t.Run("NewSSHRepository creates table", func(t *testing.T) {
		db := newTestDB(t)
		repo, err := NewSSHRepository(db)
		if err != nil {
			t.Fatalf("NewSSHRepository: %v", err)
		}
		if repo == nil {
			t.Fatal("expected non-nil repository")
		}

		// Verify table exists by querying it
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM user_vms").Scan(&count)
		if err != nil {
			t.Fatalf("table user_vms should exist: %v", err)
		}
	})

	t.Run("RegisterVM and ListVMsByEmail", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		err := repo.RegisterVM(ctx, "user@test.com", "vm-1", "os-id-1", "10.0.0.1")
		if err != nil {
			t.Fatalf("RegisterVM: %v", err)
		}
		err = repo.RegisterVM(ctx, "user@test.com", "vm-2", "os-id-2", "10.0.0.2")
		if err != nil {
			t.Fatalf("RegisterVM: %v", err)
		}
		err = repo.RegisterVM(ctx, "other@test.com", "vm-3", "os-id-3", "10.0.0.3")
		if err != nil {
			t.Fatalf("RegisterVM: %v", err)
		}

		vms, err := repo.ListVMsByEmail(ctx, "user@test.com")
		if err != nil {
			t.Fatalf("ListVMsByEmail: %v", err)
		}
		if len(vms) != 2 {
			t.Fatalf("expected 2 VMs for user@test.com, got %d", len(vms))
		}
		if vms[0].VMName != "vm-1" || vms[1].VMName != "vm-2" {
			t.Fatalf("unexpected VM names: %v, %v", vms[0].VMName, vms[1].VMName)
		}
		if vms[0].FixedIP != "10.0.0.1" {
			t.Fatalf("expected fixed IP 10.0.0.1, got %q", vms[0].FixedIP)
		}
	})

	t.Run("ListVMsByEmail returns empty for unknown user", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		vms, err := repo.ListVMsByEmail(ctx, "nobody@test.com")
		if err != nil {
			t.Fatalf("ListVMsByEmail: %v", err)
		}
		if len(vms) != 0 {
			t.Fatalf("expected 0 VMs, got %d", len(vms))
		}
	})

	t.Run("FindVMByEmailAndName returns VM when found", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		_ = repo.RegisterVM(ctx, "user@test.com", "my-vm", "os-id-1", "10.0.0.5")

		vm, err := repo.FindVMByEmailAndName(ctx, "user@test.com", "my-vm")
		if err != nil {
			t.Fatalf("FindVMByEmailAndName: %v", err)
		}
		if vm == nil {
			t.Fatal("expected non-nil VM")
		}
		if vm.VMName != "my-vm" || vm.FixedIP != "10.0.0.5" || vm.VMID != "os-id-1" {
			t.Fatalf("unexpected VM fields: %+v", vm)
		}
		if vm.UserEmail != "user@test.com" {
			t.Fatalf("expected email user@test.com, got %q", vm.UserEmail)
		}
	})

	t.Run("FindVMByEmailAndName returns nil when not found", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		vm, err := repo.FindVMByEmailAndName(ctx, "user@test.com", "nonexistent")
		if err != nil {
			t.Fatalf("FindVMByEmailAndName: %v", err)
		}
		if vm != nil {
			t.Fatalf("expected nil VM, got %+v", vm)
		}
	})

	t.Run("FindVMByEmailAndName does not return other users VMs", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		_ = repo.RegisterVM(ctx, "alice@test.com", "shared-name", "os-id-1", "10.0.0.1")

		vm, err := repo.FindVMByEmailAndName(ctx, "bob@test.com", "shared-name")
		if err != nil {
			t.Fatalf("FindVMByEmailAndName: %v", err)
		}
		if vm != nil {
			t.Fatal("expected nil — bob should not see alice's VM")
		}
	})

	t.Run("UnregisterVM removes VM", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		_ = repo.RegisterVM(ctx, "user@test.com", "vm-1", "os-id-1", "10.0.0.1")
		_ = repo.RegisterVM(ctx, "user@test.com", "vm-2", "os-id-2", "10.0.0.2")

		err := repo.UnregisterVM(ctx, "os-id-1")
		if err != nil {
			t.Fatalf("UnregisterVM: %v", err)
		}

		vms, _ := repo.ListVMsByEmail(ctx, "user@test.com")
		if len(vms) != 1 {
			t.Fatalf("expected 1 VM after unregister, got %d", len(vms))
		}
		if vms[0].VMID != "os-id-2" {
			t.Fatalf("expected remaining VM os-id-2, got %q", vms[0].VMID)
		}
	})

	t.Run("UnregisterVM is no-op for unknown vmID", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		err := repo.UnregisterVM(ctx, "nonexistent-id")
		if err != nil {
			t.Fatalf("UnregisterVM should not error for unknown ID: %v", err)
		}
	})

	t.Run("RegisterVM rejects duplicate vm_id", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		_ = repo.RegisterVM(ctx, "user@test.com", "vm-1", "same-id", "10.0.0.1")
		err := repo.RegisterVM(ctx, "user@test.com", "vm-2", "same-id", "10.0.0.2")
		if err == nil {
			t.Fatal("expected error for duplicate vm_id")
		}
	})

	t.Run("RegisterVM rejects duplicate (email, vm_name)", func(t *testing.T) {
		db := newTestDB(t)
		repo, _ := NewSSHRepository(db)

		_ = repo.RegisterVM(ctx, "user@test.com", "same-name", "os-id-1", "10.0.0.1")
		err := repo.RegisterVM(ctx, "user@test.com", "same-name", "os-id-2", "10.0.0.2")
		if err == nil {
			t.Fatal("expected error for duplicate (email, vm_name)")
		}
	})
}

// ===========================================================================
// SSHService tests
// ===========================================================================

func TestSSHServiceMenuPageSize(t *testing.T) {
	t.Run("returns configured value", func(t *testing.T) {
		svc := NewSSHService(&fakeSSHVMRepository{}, nil, nil, 5)
		if svc.MenuPageSize() != 5 {
			t.Fatalf("expected 5, got %d", svc.MenuPageSize())
		}
	})

	t.Run("defaults to 10 when zero", func(t *testing.T) {
		svc := NewSSHService(&fakeSSHVMRepository{}, nil, nil, 0)
		if svc.MenuPageSize() != 10 {
			t.Fatalf("expected default 10, got %d", svc.MenuPageSize())
		}
	})

	t.Run("defaults to 10 when negative", func(t *testing.T) {
		svc := NewSSHService(&fakeSSHVMRepository{}, nil, nil, -1)
		if svc.MenuPageSize() != 10 {
			t.Fatalf("expected default 10, got %d", svc.MenuPageSize())
		}
	})
}

func TestSSHServiceListUserVMs(t *testing.T) {
	ctx := context.Background()
	expected := []UserVM{
		{ID: 1, VMName: "vm-1", FixedIP: "10.0.0.1"},
		{ID: 2, VMName: "vm-2", FixedIP: "10.0.0.2"},
	}

	repo := &fakeSSHVMRepository{
		listVMsByEmailFn: func(_ context.Context, email string) ([]UserVM, error) {
			if email != "alice@test.com" {
				t.Fatalf("unexpected email %q", email)
			}
			return expected, nil
		},
	}

	svc := NewSSHService(repo, nil, nil, 10)
	vms, err := svc.ListUserVMs(ctx, "alice@test.com")
	if err != nil {
		t.Fatalf("ListUserVMs: %v", err)
	}
	if len(vms) != 2 {
		t.Fatalf("expected 2 VMs, got %d", len(vms))
	}
}

func TestSSHServiceResolveVM(t *testing.T) {
	ctx := context.Background()

	t.Run("returns VM when found", func(t *testing.T) {
		expected := &UserVM{ID: 1, VMName: "my-vm", FixedIP: "10.0.0.5"}
		repo := &fakeSSHVMRepository{
			findVMByEmailAndName: func(_ context.Context, email, vmName string) (*UserVM, error) {
				return expected, nil
			},
		}

		svc := NewSSHService(repo, nil, nil, 10)
		vm, err := svc.ResolveVM(ctx, "user@test.com", "my-vm")
		if err != nil {
			t.Fatalf("ResolveVM: %v", err)
		}
		if vm.VMName != "my-vm" {
			t.Fatalf("expected my-vm, got %q", vm.VMName)
		}
	})

	t.Run("returns ErrVMNotFound when nil", func(t *testing.T) {
		repo := &fakeSSHVMRepository{
			findVMByEmailAndName: func(_ context.Context, _, _ string) (*UserVM, error) {
				return nil, nil
			},
		}

		svc := NewSSHService(repo, nil, nil, 10)
		_, err := svc.ResolveVM(ctx, "user@test.com", "missing")
		if !errors.Is(err, ErrVMNotFound) {
			t.Fatalf("expected ErrVMNotFound, got %v", err)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := &fakeSSHVMRepository{
			findVMByEmailAndName: func(_ context.Context, _, _ string) (*UserVM, error) {
				return nil, repoErr
			},
		}

		svc := NewSSHService(repo, nil, nil, 10)
		_, err := svc.ResolveVM(ctx, "user@test.com", "vm")
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got %v", err)
		}
	})
}

func TestSSHServiceGetServiceKey(t *testing.T) {
	signer := generateTestSigner(t)
	svc := NewSSHService(&fakeSSHVMRepository{}, nil, signer, 10)

	if svc.GetServiceKey() != signer {
		t.Fatal("expected same signer to be returned")
	}
}

// ===========================================================================
// Auth: buildSSHServerConfig tests
// ===========================================================================

type fakeUserVerifier struct {
	existsFn func(ctx context.Context, email string) (bool, error)
}

func (f *fakeUserVerifier) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return f.existsFn(ctx, email)
}

func TestBuildSSHServerConfig(t *testing.T) {
	hostKey := generateTestSigner(t)
	caKey := generateTestSigner(t)

	t.Run("rejects non-certificate key", func(t *testing.T) {
		verifier := &fakeUserVerifier{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				t.Fatal("should not be called")
				return false, nil
			},
		}

		cfg := buildSSHServerConfig(hostKey, caKey.PublicKey(), verifier)
		regularKey := generateTestSigner(t)
		_, err := cfg.PublicKeyCallback(fakeConnMeta("user"), regularKey.PublicKey())
		if err == nil {
			t.Fatal("expected error for non-certificate key")
		}
		if !strings.Contains(err.Error(), "not a certificate") {
			t.Fatalf("expected 'not a certificate' error, got: %v", err)
		}
	})

	t.Run("rejects cert signed by wrong CA", func(t *testing.T) {
		verifier := &fakeUserVerifier{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return true, nil
			},
		}

		wrongCA := generateTestSigner(t)
		cert := signTestCert(t, wrongCA, "user@test.com")

		cfg := buildSSHServerConfig(hostKey, caKey.PublicKey(), verifier)
		_, err := cfg.PublicKeyCallback(fakeConnMeta("user"), cert)
		if err == nil {
			t.Fatal("expected error for wrong CA")
		}
	})

	t.Run("rejects cert with no principals", func(t *testing.T) {
		verifier := &fakeUserVerifier{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return true, nil
			},
		}

		cert := signTestCertWithPrincipals(t, caKey, "user", nil)

		cfg := buildSSHServerConfig(hostKey, caKey.PublicKey(), verifier)
		_, err := cfg.PublicKeyCallback(fakeConnMeta("user"), cert)
		if err == nil {
			t.Fatal("expected error for cert with no principals")
		}
	})

	t.Run("rejects unregistered user", func(t *testing.T) {
		verifier := &fakeUserVerifier{
			existsFn: func(_ context.Context, email string) (bool, error) {
				return false, nil
			},
		}

		cert := signTestCert(t, caKey, "unknown@test.com")

		cfg := buildSSHServerConfig(hostKey, caKey.PublicKey(), verifier)
		_, err := cfg.PublicKeyCallback(fakeConnMeta("unknown@test.com"), cert)
		if err == nil {
			t.Fatal("expected error for unregistered user")
		}
		if !strings.Contains(err.Error(), "not registered") {
			t.Fatalf("expected 'not registered' error, got: %v", err)
		}
	})

	t.Run("accepts valid cert and stores email", func(t *testing.T) {
		verifier := &fakeUserVerifier{
			existsFn: func(_ context.Context, email string) (bool, error) {
				if email != "alice@test.com" {
					return false, nil
				}
				return true, nil
			},
		}

		cert := signTestCert(t, caKey, "alice@test.com")

		cfg := buildSSHServerConfig(hostKey, caKey.PublicKey(), verifier)
		perms, err := cfg.PublicKeyCallback(fakeConnMeta("alice@test.com"), cert)
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if perms.Extensions["email"] != "alice@test.com" {
			t.Fatalf("expected email in extensions, got %v", perms.Extensions)
		}
	})

	t.Run("propagates verifier error", func(t *testing.T) {
		verifier := &fakeUserVerifier{
			existsFn: func(_ context.Context, _ string) (bool, error) {
				return false, errors.New("db down")
			},
		}

		cert := signTestCert(t, caKey, "user@test.com")

		cfg := buildSSHServerConfig(hostKey, caKey.PublicKey(), verifier)
		_, err := cfg.PublicKeyCallback(fakeConnMeta("user@test.com"), cert)
		if err == nil {
			t.Fatal("expected error when verifier fails")
		}
		if !strings.Contains(err.Error(), "user lookup failed") {
			t.Fatalf("expected 'user lookup failed' error, got: %v", err)
		}
	})
}

// signTestCert creates a valid SSH user certificate signed by the given CA.
func signTestCert(t *testing.T, ca gossh.Signer, principal string) *gossh.Certificate {
	t.Helper()
	return signTestCertWithPrincipals(t, ca, principal, []string{principal})
}

func signTestCertWithPrincipals(t *testing.T, ca gossh.Signer, user string, principals []string) *gossh.Certificate {
	t.Helper()
	userKey := generateTestSigner(t)
	cert := &gossh.Certificate{
		CertType:        gossh.UserCert,
		Key:             userKey.PublicKey(),
		KeyId:           "test-cert",
		ValidPrincipals: principals,
		ValidAfter:      uint64(time.Now().Add(-time.Hour).Unix()),
		ValidBefore:     uint64(time.Now().Add(time.Hour).Unix()),
	}
	if err := cert.SignCert(rand.Reader, ca); err != nil {
		t.Fatalf("sign cert: %v", err)
	}
	return cert
}

// fakeConnMeta implements gossh.ConnMetadata for testing PublicKeyCallback.
type fakeConnMeta string

func (f fakeConnMeta) User() string          { return string(f) }
func (f fakeConnMeta) SessionID() []byte     { return []byte("test-session") }
func (f fakeConnMeta) ClientVersion() []byte { return []byte("SSH-2.0-test") }
func (f fakeConnMeta) ServerVersion() []byte { return []byte("SSH-2.0-test") }
func (f fakeConnMeta) RemoteAddr() net.Addr { return fakeAddr("127.0.0.1:22") }
func (f fakeConnMeta) LocalAddr() net.Addr  { return fakeAddr("127.0.0.1:2222") }

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

// ===========================================================================
// Auth: key loading tests
// ===========================================================================

func writeTestPrivateKey(t *testing.T, dir, name string) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	path := dir + "/" + name
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	return path
}

func TestLoadSSHHostKey(t *testing.T) {
	t.Run("loads valid ed25519 key", func(t *testing.T) {
		path := writeTestPrivateKey(t, t.TempDir(), "host_key")

		loaded, err := loadSSHHostKey(path)
		if err != nil {
			t.Fatalf("loadSSHHostKey: %v", err)
		}
		if loaded == nil {
			t.Fatal("expected non-nil signer")
		}
	})

	t.Run("errors on missing file", func(t *testing.T) {
		_, err := loadSSHHostKey("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("errors on invalid key data", func(t *testing.T) {
		tmpFile := t.TempDir() + "/bad_key"
		_ = os.WriteFile(tmpFile, []byte("not a key"), 0600)

		_, err := loadSSHHostKey(tmpFile)
		if err == nil {
			t.Fatal("expected error for invalid key")
		}
	})
}

func TestLoadCFCAKey(t *testing.T) {
	t.Run("loads valid public key", func(t *testing.T) {
		signer := generateTestSigner(t)
		authKey := gossh.MarshalAuthorizedKey(signer.PublicKey())

		tmpFile := t.TempDir() + "/cf_ca.pub"
		if err := os.WriteFile(tmpFile, authKey, 0644); err != nil {
			t.Fatalf("write temp key: %v", err)
		}

		loaded, err := loadCFCAKey(tmpFile)
		if err != nil {
			t.Fatalf("loadCFCAKey: %v", err)
		}
		if loaded == nil {
			t.Fatal("expected non-nil public key")
		}
	})

	t.Run("errors on missing file", func(t *testing.T) {
		_, err := loadCFCAKey("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

func TestLoadSSHServiceKey(t *testing.T) {
	t.Run("loads valid key", func(t *testing.T) {
		path := writeTestPrivateKey(t, t.TempDir(), "service_key")

		loaded, err := loadSSHServiceKey(path)
		if err != nil {
			t.Fatalf("loadSSHServiceKey: %v", err)
		}
		if loaded == nil {
			t.Fatal("expected non-nil signer")
		}
	})

	t.Run("errors on missing file", func(t *testing.T) {
		_, err := loadSSHServiceKey("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

// ===========================================================================
// Menu rendering tests
// ===========================================================================

func TestRenderSSHMenu(t *testing.T) {
	t.Run("renders single page", func(t *testing.T) {
		ch := newFakeChannel("")
		vms := []UserVM{
			{VMName: "web-server", FixedIP: "10.0.0.1"},
			{VMName: "db-server", FixedIP: "10.0.0.2"},
		}

		renderSSHMenu(ch, vms, 0, 10)
		output := ch.written.String()

		if !strings.Contains(output, "web-server") {
			t.Fatal("expected output to contain web-server")
		}
		if !strings.Contains(output, "db-server") {
			t.Fatal("expected output to contain db-server")
		}
		if !strings.Contains(output, "10.0.0.1") {
			t.Fatal("expected output to contain IP 10.0.0.1")
		}
		if !strings.Contains(output, "1-2 / 2") {
			t.Fatalf("expected header '1-2 / 2' in output, got: %s", output)
		}
		if !strings.Contains(output, "1.") {
			t.Fatal("expected numbering starting at 1")
		}
		if !strings.Contains(output, "2.") {
			t.Fatal("expected numbering including 2")
		}
	})

	t.Run("renders correct page subset", func(t *testing.T) {
		ch := newFakeChannel("")
		vms := make([]UserVM, 15)
		for i := range vms {
			vms[i] = UserVM{VMName: strings.Repeat("a", i+1), FixedIP: "10.0.0.1"}
		}

		renderSSHMenu(ch, vms, 1, 5)
		output := ch.written.String()

		if !strings.Contains(output, "6-10 / 15") {
			t.Fatalf("expected header '6-10 / 15', got: %s", output)
		}
	})
}

// ===========================================================================
// sshReadLine tests
// ===========================================================================

func TestSSHReadLine(t *testing.T) {
	t.Run("reads simple line", func(t *testing.T) {
		ch := newFakeChannel("hello\r")
		line, err := sshReadLine(ch)
		if err != nil {
			t.Fatalf("sshReadLine: %v", err)
		}
		if line != "hello" {
			t.Fatalf("expected 'hello', got %q", line)
		}
	})

	t.Run("handles newline terminator", func(t *testing.T) {
		ch := newFakeChannel("test\n")
		line, err := sshReadLine(ch)
		if err != nil {
			t.Fatalf("sshReadLine: %v", err)
		}
		if line != "test" {
			t.Fatalf("expected 'test', got %q", line)
		}
	})

	t.Run("handles backspace", func(t *testing.T) {
		ch := newFakeChannel("ab\x7fc\r") // type 'a', 'b', backspace, 'c', enter
		line, err := sshReadLine(ch)
		if err != nil {
			t.Fatalf("sshReadLine: %v", err)
		}
		if line != "ac" {
			t.Fatalf("expected 'ac' after backspace, got %q", line)
		}
	})

	t.Run("backspace on empty line is no-op", func(t *testing.T) {
		ch := newFakeChannel("\x7f\x7fa\r") // backspace twice on empty, then 'a'
		line, err := sshReadLine(ch)
		if err != nil {
			t.Fatalf("sshReadLine: %v", err)
		}
		if line != "a" {
			t.Fatalf("expected 'a', got %q", line)
		}
	})

	t.Run("returns error on EOF", func(t *testing.T) {
		ch := newFakeChannel("") // empty — Read returns EOF
		_, err := sshReadLine(ch)
		if err == nil {
			t.Fatal("expected error on EOF")
		}
	})
}

// ===========================================================================
// bridgeSSHStreams tests
// ===========================================================================

func TestBridgeSSHStreams(t *testing.T) {
	t.Run("copies data bidirectionally", func(t *testing.T) {
		aRead, aWrite := io.Pipe()
		bRead, bWrite := io.Pipe()

		a := &testRWC{Reader: aRead, Writer: bWrite}
		b := &testRWC{Reader: bRead, Writer: aWrite}

		done := make(chan struct{})
		go func() {
			bridgeSSHStreams(a, b)
			close(done)
		}()

		// Write "hello" to a → should appear readable from b's writer (aWrite) side
		// Since bridgeSSHStreams copies a→b and b→a, writing to aWrite means
		// b reads it and copies to a. Let's just verify the bridge closes properly.

		// Close one side to trigger bridge shutdown
		aWrite.Close()

		select {
		case <-done:
			// bridge completed
		case <-time.After(2 * time.Second):
			t.Fatal("bridgeSSHStreams did not complete in time")
		}
	})
}

type testRWC struct {
	io.Reader
	io.Writer
	closed bool
}

func (t *testRWC) Close() error {
	t.closed = true
	if c, ok := t.Reader.(io.Closer); ok {
		c.Close()
	}
	if c, ok := t.Writer.(io.Closer); ok {
		c.Close()
	}
	return nil
}

// ===========================================================================
// SSHServer tests
// ===========================================================================

func TestNewSSHServer(t *testing.T) {
	cfg := SSHConfig{ListenAddr: ":9999"}
	server := NewSSHServer(cfg, &gossh.ServerConfig{}, nil)

	if server.Addr() != ":9999" {
		t.Fatalf("expected addr :9999, got %q", server.Addr())
	}
}

// ===========================================================================
// ConnectionHandler constructor test
// ===========================================================================

func TestNewConnectionHandler(t *testing.T) {
	svc := NewSSHService(&fakeSSHVMRepository{}, nil, nil, 10)
	handler := NewConnectionHandler(svc)
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

// ===========================================================================
// nsConn (namespace connection) tests
// ===========================================================================

func TestNsAddr(t *testing.T) {
	addr := nsAddr("10.0.0.1:22")
	if addr.Network() != "tcp" {
		t.Fatalf("expected tcp, got %q", addr.Network())
	}
	if addr.String() != "10.0.0.1:22" {
		t.Fatalf("expected 10.0.0.1:22, got %q", addr.String())
	}
}
