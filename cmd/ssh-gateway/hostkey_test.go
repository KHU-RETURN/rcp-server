package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh/knownhosts"
)

func TestReloadingInnerHostKeyCallbackTrustsUnknownHostOnFirstUse(t *testing.T) {
	hostKeyPath := filepath.Join(t.TempDir(), "host_ed25519")
	signer, err := LoadOrCreateHostKey(hostKeyPath)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "missing_known_hosts")
	cb := reloadingInnerHostKeyCallback(knownHostsPath)

	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, signer.PublicKey()); err != nil {
		t.Fatalf("unknown host should be trusted on first use: %v", err)
	}
	raw, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	if got := string(raw); !strings.Contains(got, "10.0.0.7 ") {
		t.Fatalf("known_hosts = %q, want normalized host entry", got)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, signer.PublicKey()); err != nil {
		t.Fatalf("stored host key rejected: %v", err)
	}
}

func TestLoadInnerHostKeyCallbackAcceptsKnownHost(t *testing.T) {
	hostKeyPath := filepath.Join(t.TempDir(), "host_ed25519")
	signer, err := LoadOrCreateHostKey(hostKeyPath)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"10.0.0.7"}, signer.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	cb, err := loadInnerHostKeyCallback(knownHostsPath)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, signer.PublicKey()); err != nil {
		t.Fatalf("known host rejected: %v", err)
	}

	otherSigner, err := LoadOrCreateHostKey(filepath.Join(t.TempDir(), "other_ed25519"))
	if err != nil {
		t.Fatalf("other host key: %v", err)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, otherSigner.PublicKey()); err == nil {
		t.Fatal("expected mismatched host key to be rejected")
	}
}

func TestInnerHostKeyCallbackForAddressIgnoresProxyRemoteAddr(t *testing.T) {
	hostKeyPath := filepath.Join(t.TempDir(), "host_ed25519")
	signer, err := LoadOrCreateHostKey(hostKeyPath)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"10.0.0.7"}, signer.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	cb, err := loadInnerHostKeyCallback(knownHostsPath)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	wrapped := innerHostKeyCallbackForAddress("10.0.0.7:22", cb)
	if err := wrapped("", &net.UnixAddr{Name: "/run/rcp/ns-proxy.sock", Net: "unix"}, signer.PublicKey()); err != nil {
		t.Fatalf("known host rejected through proxy remote addr: %v", err)
	}
}

func TestReloadingInnerHostKeyCallbackPicksUpCreatedFile(t *testing.T) {
	hostKeyPath := filepath.Join(t.TempDir(), "host_ed25519")
	signer, err := LoadOrCreateHostKey(hostKeyPath)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	cb := reloadingInnerHostKeyCallback(knownHostsPath)

	line := knownhosts.Line([]string{"10.0.0.7"}, signer.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, signer.PublicKey()); err != nil {
		t.Fatalf("known host rejected after reload: %v", err)
	}
}

func TestReloadingInnerHostKeyCallbackRejectsChangedHostKey(t *testing.T) {
	hostKeyPath := filepath.Join(t.TempDir(), "host_ed25519")
	signer, err := LoadOrCreateHostKey(hostKeyPath)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"10.0.0.7"}, signer.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	otherSigner, err := LoadOrCreateHostKey(filepath.Join(t.TempDir(), "other_ed25519"))
	if err != nil {
		t.Fatalf("other host key: %v", err)
	}
	cb := reloadingInnerHostKeyCallback(knownHostsPath)
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, otherSigner.PublicKey()); err == nil {
		t.Fatal("expected changed host key to be rejected")
	}
}
