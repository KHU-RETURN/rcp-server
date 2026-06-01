package access

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestLoadVMHostKeyCallbackFailsClosedWhenMissing(t *testing.T) {
	_, err := loadVMHostKeyCallback(filepath.Join(t.TempDir(), "missing_known_hosts"))
	if err == nil {
		t.Fatal("expected missing known_hosts to fail")
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

func TestReloadingVMHostKeyCallbackPicksUpCreatedFile(t *testing.T) {
	pub, err := testHostPublicKey()
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	cb := reloadingVMHostKeyCallback(knownHostsPath)

	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, pub); err == nil {
		t.Fatal("expected missing known_hosts to fail before reload")
	}

	line := knownhosts.Line([]string{"10.0.0.7"}, pub)
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, pub); err != nil {
		t.Fatalf("known host rejected after reload: %v", err)
	}
}

func testHostPublicKey() (ssh.PublicKey, error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return ssh.NewPublicKey(pub)
}
