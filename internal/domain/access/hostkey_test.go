package access

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestReloadingVMHostKeyCallbackTrustsUnknownHostOnFirstUse(t *testing.T) {
	pub, err := testHostPublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "missing_known_hosts")
	cb := reloadingVMHostKeyCallback(knownHostsPath)

	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, pub); err != nil {
		t.Fatalf("unknown host should be trusted on first use: %v", err)
	}
	raw, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if got := string(raw); !strings.Contains(got, "10.0.0.7 ") {
		t.Fatalf("known_hosts = %q, want normalized host entry", got)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, pub); err != nil {
		t.Fatalf("stored host key rejected: %v", err)
	}
}

func TestLoadVMHostKeyCallbackAcceptsKnownHost(t *testing.T) {
	pub, err := testHostPublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"10.0.0.7"}, pub)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	cb, err := loadVMHostKeyCallback(knownHostsPath)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, pub); err != nil {
		t.Fatalf("known host rejected: %v", err)
	}
}

func TestVMHostKeyCallbackForAddressIgnoresProxyRemoteAddr(t *testing.T) {
	pub, err := testHostPublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"10.0.0.7"}, pub)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	cb, err := loadVMHostKeyCallback(knownHostsPath)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	wrapped := vmHostKeyCallbackForAddress("10.0.0.7:22", cb)
	if err := wrapped("", &net.UnixAddr{Name: "/run/rcp/ns-proxy.sock", Net: "unix"}, pub); err != nil {
		t.Fatalf("known host rejected through proxy remote addr: %v", err)
	}
}

func TestReloadingVMHostKeyCallbackPicksUpCreatedFile(t *testing.T) {
	pub, err := testHostPublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	cb := reloadingVMHostKeyCallback(knownHostsPath)

	line := knownhosts.Line([]string{"10.0.0.7"}, pub)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, pub); err != nil {
		t.Fatalf("known host rejected after reload: %v", err)
	}
}

func TestReloadingVMHostKeyCallbackRejectsChangedHostKey(t *testing.T) {
	pub, err := testHostPublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"10.0.0.7"}, pub)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	otherPub, err := testHostPublicKey()
	if err != nil {
		t.Fatalf("other public key: %v", err)
	}
	cb := reloadingVMHostKeyCallback(knownHostsPath)
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, otherPub); err == nil {
		t.Fatal("expected changed host key to be rejected")
	}
}

func testHostPublicKey() (ssh.PublicKey, error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return ssh.NewPublicKey(pub)
}
