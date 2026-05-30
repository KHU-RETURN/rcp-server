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
	if cfg.DBDriver != "sqlite3" {
		t.Errorf("DBDriver default: got %q", cfg.DBDriver)
	}
	if cfg.DBDSN != "file:/var/lib/rcp/rcp.db?mode=ro&cache=shared&_pragma=foreign_keys(1)" {
		t.Errorf("DBDSN fallback: got %q", cfg.DBDSN)
	}
	if cfg.KnownHostsPath != "/etc/rcp/ssh-gateway/known_hosts" {
		t.Errorf("KnownHostsPath default: got %q", cfg.KnownHostsPath)
	}
	if cfg.MaxPendingSessions != defaultMaxPendingSessions {
		t.Errorf("MaxPendingSessions default: got %d", cfg.MaxPendingSessions)
	}
	if cfg.FixedNetworkName != "" {
		t.Errorf("FixedNetworkName default: got %q", cfg.FixedNetworkName)
	}
	if cfg.VMUser != "root" {
		t.Errorf("VMUser default: got %q", cfg.VMUser)
	}
}

func TestLoadConfig_AllowsDBDSNWithoutLegacyDBPath(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"RCP_SSH_GW_NOTIFY_SECRET": "abc123",
		"RCP_SSH_GW_AUTH_URL_BASE": "https://rcp.return.dev",
		"DB_DRIVER":                "postgres",
		"DB_DSN":                   "host=db dbname=rcp",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.DBDriver != "postgres" || cfg.DBDSN != "host=db dbname=rcp" {
		t.Fatalf("got driver=%q dsn=%q", cfg.DBDriver, cfg.DBDSN)
	}
}

func TestLoadConfig_AllowsAbsoluteSQLiteDBDSN(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"RCP_SSH_GW_NOTIFY_SECRET": "abc123",
		"RCP_SSH_GW_AUTH_URL_BASE": "https://rcp.return.dev",
		"DB_DSN":                   "file:/var/lib/rcp/custom.db?cache=shared",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.DBDSN != "file:/var/lib/rcp/custom.db?cache=shared" {
		t.Fatalf("dsn got %q", cfg.DBDSN)
	}
}

func TestLoadConfig_RejectsRelativeSQLiteDBDSN(t *testing.T) {
	for _, dsn := range []string{"file:rcp.db?cache=shared", "rcp.db"} {
		t.Run(dsn, func(t *testing.T) {
			_, err := LoadConfig(envFromMap(map[string]string{
				"RCP_SSH_GW_NOTIFY_SECRET": "abc123",
				"RCP_SSH_GW_AUTH_URL_BASE": "https://rcp.return.dev",
				"DB_DSN":                   dsn,
			}))
			if err == nil || !strings.Contains(err.Error(), "DB_DSN") {
				t.Fatalf("expected DB_DSN error, got %v", err)
			}
		})
	}
}

func TestLoadConfig_RequiredMissing(t *testing.T) {
	cases := []struct {
		drop string
		want string
	}{
		{"RCP_SSH_GW_NOTIFY_SECRET", "RCP_SSH_GW_NOTIFY_SECRET"},
		{"RCP_SSH_GW_AUTH_URL_BASE", "RCP_SSH_GW_AUTH_URL_BASE"},
		{"RCP_SSH_GW_DB_PATH", "RCP_SSH_GW_DB_PATH or DB_DSN"},
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

func TestLoadConfig_MaxPendingPositive(t *testing.T) {
	_, err := LoadConfig(envFromMap(map[string]string{
		"RCP_SSH_GW_NOTIFY_SECRET":        "x",
		"RCP_SSH_GW_AUTH_URL_BASE":        "x",
		"RCP_SSH_GW_DB_PATH":              "/x",
		"RCP_SSH_GW_MAX_PENDING_SESSIONS": "0",
	}))
	if err == nil {
		t.Fatalf("expected MAX_PENDING_SESSIONS > 0 error")
	}
}

func TestLoadConfig_FixedNetworkName(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"RCP_SSH_GW_NOTIFY_SECRET": "abc123",
		"RCP_SSH_GW_AUTH_URL_BASE": "https://rcp.return.dev",
		"RCP_SSH_GW_DB_PATH":       "/var/lib/rcp/rcp.db",
		"RCP_SSH_GW_FIXED_NETWORK": " tenant-a ",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.FixedNetworkName != "tenant-a" {
		t.Fatalf("got %q", cfg.FixedNetworkName)
	}
}

func TestLoadConfig_VMUser(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"RCP_SSH_GW_NOTIFY_SECRET": "abc123",
		"RCP_SSH_GW_AUTH_URL_BASE": "https://rcp.return.dev",
		"RCP_SSH_GW_DB_PATH":       "/var/lib/rcp/rcp.db",
		"RCP_SSH_GW_VM_USER":       " ubuntu ",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.VMUser != "ubuntu" {
		t.Fatalf("got %q", cfg.VMUser)
	}
}
