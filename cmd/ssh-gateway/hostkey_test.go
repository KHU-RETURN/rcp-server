package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh/knownhosts"
)

func TestLoadInnerHostKeyCallbackFailsClosedWhenMissing(t *testing.T) {
	_, err := loadInnerHostKeyCallback(filepath.Join(t.TempDir(), "missing_known_hosts"))
	if err == nil {
		t.Fatal("expected missing known_hosts to fail")
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

func TestReloadingInnerHostKeyCallbackPicksUpCreatedFile(t *testing.T) {
	hostKeyPath := filepath.Join(t.TempDir(), "host_ed25519")
	signer, err := LoadOrCreateHostKey(hostKeyPath)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	cb := reloadingInnerHostKeyCallback(knownHostsPath)

	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, signer.PublicKey()); err == nil {
		t.Fatal("expected missing known_hosts to fail before reload")
	}

	line := knownhosts.Line([]string{"10.0.0.7"}, signer.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}
	if err := cb("10.0.0.7:22", &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 22}, signer.PublicKey()); err != nil {
		t.Fatalf("known host rejected after reload: %v", err)
	}
}
