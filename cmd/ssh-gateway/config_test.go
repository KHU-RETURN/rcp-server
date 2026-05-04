package main

import (
	"strings"
	"testing"
	"time"
)

func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfig_AllDefaultsExceptRequired(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"RCP_SSH_GW_NOTIFY_SECRET": "abc123",
		"RCP_SSH_GW_AUTH_URL_BASE": "https://rcp.return.dev",
		"RCP_SSH_GW_DB_PATH":       "/var/lib/rcp/rcp.db",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Listen != "127.0.0.1:2222" {
		t.Errorf("Listen default: got %q want 127.0.0.1:2222", cfg.Listen)
	}
	if cfg.HostKeyPath != "/etc/rcp/ssh-gateway/host_ed25519" {
		t.Errorf("HostKeyPath default: got %q", cfg.HostKeyPath)
	}
	if cfg.NotifySock != "/run/rcp/ssh-gateway-notify.sock" {
		t.Errorf("NotifySock default: got %q", cfg.NotifySock)
	}
	if cfg.NsProxySock != "/run/rcp/ns-proxy.sock" {
		t.Errorf("NsProxySock default: got %q", cfg.NsProxySock)
	}
	if cfg.NonceTTL != 5*time.Minute {
		t.Errorf("NonceTTL default: got %v", cfg.NonceTTL)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default: got %q", cfg.LogLevel)
	}
}

func TestLoadConfig_RequiredMissing(t *testing.T) {
	cases := []struct {
		drop string
		want string
	}{
		{"RCP_SSH_GW_NOTIFY_SECRET", "RCP_SSH_GW_NOTIFY_SECRET"},
		{"RCP_SSH_GW_AUTH_URL_BASE", "RCP_SSH_GW_AUTH_URL_BASE"},
		{"RCP_SSH_GW_DB_PATH", "RCP_SSH_GW_DB_PATH"},
	}
	for _, tc := range cases {
		t.Run(tc.drop, func(t *testing.T) {
			env := map[string]string{
				"RCP_SSH_GW_NOTIFY_SECRET": "abc",
				"RCP_SSH_GW_AUTH_URL_BASE": "https://x",
				"RCP_SSH_GW_DB_PATH":       "/x",
			}
			delete(env, tc.drop)
			_, err := LoadConfig(envFromMap(env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

func TestLoadConfig_NonceTTLPositive(t *testing.T) {
	_, err := LoadConfig(envFromMap(map[string]string{
		"RCP_SSH_GW_NOTIFY_SECRET": "x",
		"RCP_SSH_GW_AUTH_URL_BASE": "x",
		"RCP_SSH_GW_DB_PATH":       "x",
		"RCP_SSH_GW_NONCE_TTL":     "0s",
	}))
	if err == nil {
		t.Fatalf("expected NONCE_TTL > 0 error")
	}
}
