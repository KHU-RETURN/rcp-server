package main

import (
	"testing"
	"time"
)

func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Listen != ":18080" {
		t.Fatalf("Listen got %q", cfg.Listen)
	}
	if cfg.NsProxySock != "/run/rcp/ns-proxy.sock" {
		t.Fatalf("NsProxySock got %q", cfg.NsProxySock)
	}
	if cfg.DBDriver != "sqlite3" {
		t.Fatalf("DBDriver got %q", cfg.DBDriver)
	}
	if cfg.DBDSN != "file:rcp.db?mode=ro&cache=shared&_pragma=foreign_keys(1)" {
		t.Fatalf("DBDSN got %q", cfg.DBDSN)
	}
	if cfg.ReadTimeout != 30*time.Second {
		t.Fatalf("ReadTimeout got %v", cfg.ReadTimeout)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Fatalf("ShutdownTimeout got %v", cfg.ShutdownTimeout)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel got %q", cfg.LogLevel)
	}
}

func TestLoadConfigUsesExplicitValues(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"APP_GATEWAY_PORT":            "19090",
		"RCP_NS_PROXY_SOCK":           "/tmp/ns-proxy.sock",
		"DB_DRIVER":                   "postgres",
		"DB_DSN":                      "host=db dbname=rcp",
		"RCP_APP_GW_FIXED_NETWORK":    "tenant-net",
		"RCP_APP_GW_READ_TIMEOUT":     "5s",
		"RCP_APP_GW_SHUTDOWN_TIMEOUT": "7s",
		"RCP_APP_GW_LOG_LEVEL":        "debug",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Listen != ":19090" || cfg.NsProxySock != "/tmp/ns-proxy.sock" {
		t.Fatalf("unexpected listen/sock: %+v", cfg)
	}
	if cfg.DBDriver != "postgres" || cfg.DBDSN != "host=db dbname=rcp" {
		t.Fatalf("unexpected db config: %+v", cfg)
	}
	if cfg.FixedNetworkName != "tenant-net" {
		t.Fatalf("FixedNetworkName got %q", cfg.FixedNetworkName)
	}
	if cfg.ReadTimeout != 5*time.Second || cfg.ShutdownTimeout != 7*time.Second {
		t.Fatalf("unexpected timeouts: %+v", cfg)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel got %q", cfg.LogLevel)
	}
}

func TestLoadConfigAllowsHostPortGatewayPort(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"APP_GATEWAY_PORT": "127.0.0.1:19090",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.Listen != "127.0.0.1:19090" {
		t.Fatalf("Listen got %q", cfg.Listen)
	}
}

func TestLoadConfigAllowsRelativeSQLiteDSN(t *testing.T) {
	cfg, err := LoadConfig(envFromMap(map[string]string{
		"DB_DSN": "file:rcp.db?cache=shared",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if cfg.DBDSN != "file:rcp.db?cache=shared" {
		t.Fatalf("DBDSN got %q", cfg.DBDSN)
	}
}
