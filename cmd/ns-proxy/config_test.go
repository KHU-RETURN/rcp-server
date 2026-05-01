package main

import (
	"net"
	"testing"
	"time"
)

func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfig_DefaultsAndAllowCIDR(t *testing.T) {
	cfg, err := LoadConfig(mapEnv(map[string]string{
		"RCP_NS_PROXY_ALLOW_CIDR": "192.168.0.0/16",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SockPath != "/run/rcp/ns-proxy.sock" {
		t.Errorf("SockPath default = %q", cfg.SockPath)
	}
	if cfg.MaxConns != 1024 {
		t.Errorf("MaxConns default = %d", cfg.MaxConns)
	}
	if cfg.DialTimeout != 5*time.Second {
		t.Errorf("DialTimeout default = %v", cfg.DialTimeout)
	}
	if cfg.ShutdownGrace != 30*time.Second {
		t.Errorf("ShutdownGrace default = %v", cfg.ShutdownGrace)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default = %q", cfg.LogLevel)
	}
}

func TestLoadConfig_FailClosedOnMissingCIDR(t *testing.T) {
	if _, err := LoadConfig(mapEnv(map[string]string{})); err == nil {
		t.Fatal("expected error when ALLOW_CIDR missing, got nil")
	}
}

func TestLoadConfig_RejectsInvalidDuration(t *testing.T) {
	_, err := LoadConfig(mapEnv(map[string]string{
		"RCP_NS_PROXY_ALLOW_CIDR":   "192.168.0.0/16",
		"RCP_NS_PROXY_DIAL_TIMEOUT": "five-seconds",
	}))
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestLoadConfig_RejectsInvalidInt(t *testing.T) {
	_, err := LoadConfig(mapEnv(map[string]string{
		"RCP_NS_PROXY_ALLOW_CIDR": "192.168.0.0/16",
		"RCP_NS_PROXY_MAX_CONNS":  "lots",
	}))
	if err == nil {
		t.Fatal("expected error for invalid int, got nil")
	}
}

func TestLoadConfig_OverridesAll(t *testing.T) {
	cfg, err := LoadConfig(mapEnv(map[string]string{
		"RCP_NS_PROXY_SOCK":           "/tmp/x.sock",
		"RCP_NS_PROXY_ALLOW_CIDR":     "10.0.0.0/8",
		"RCP_NS_PROXY_MAX_CONNS":      "256",
		"RCP_NS_PROXY_DIAL_TIMEOUT":   "2s",
		"RCP_NS_PROXY_SHUTDOWN_GRACE": "10s",
		"RCP_NS_PROXY_LOG_LEVEL":      "debug",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SockPath != "/tmp/x.sock" || cfg.MaxConns != 256 ||
		cfg.DialTimeout != 2*time.Second || cfg.ShutdownGrace != 10*time.Second ||
		cfg.LogLevel != "debug" {
		t.Errorf("override mismatch: %+v", cfg)
	}
	if cfg.AllowList == nil || !cfg.AllowList.Contains(net.ParseIP("10.0.0.5")) {
		t.Error("AllowList override did not apply correctly")
	}
}

func TestLoadConfig_RejectsInvalidCIDR(t *testing.T) {
	_, err := LoadConfig(mapEnv(map[string]string{
		"RCP_NS_PROXY_ALLOW_CIDR": "not-a-cidr",
	}))
	if err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}

func TestLoadConfig_RejectsZeroOrNegativeMaxConns(t *testing.T) {
	for _, v := range []string{"0", "-1"} {
		_, err := LoadConfig(mapEnv(map[string]string{
			"RCP_NS_PROXY_ALLOW_CIDR": "192.168.0.0/16",
			"RCP_NS_PROXY_MAX_CONNS":  v,
		}))
		if err == nil {
			t.Errorf("MAX_CONNS=%q should error, got nil", v)
		}
	}
}

func TestLoadConfig_RejectsNonPositiveDurations(t *testing.T) {
	for _, key := range []string{"RCP_NS_PROXY_DIAL_TIMEOUT", "RCP_NS_PROXY_SHUTDOWN_GRACE"} {
		for _, v := range []string{"0s", "-1s"} {
			_, err := LoadConfig(mapEnv(map[string]string{
				"RCP_NS_PROXY_ALLOW_CIDR": "192.168.0.0/16",
				key:                       v,
			}))
			if err == nil {
				t.Errorf("%s=%q should error, got nil", key, v)
			}
		}
	}
}

func TestLoadConfig_RejectsRelativeSockPath(t *testing.T) {
	for _, v := range []string{"tmp/x.sock", "   ", "relative/path"} {
		_, err := LoadConfig(mapEnv(map[string]string{
			"RCP_NS_PROXY_ALLOW_CIDR": "192.168.0.0/16",
			"RCP_NS_PROXY_SOCK":       v,
		}))
		if err == nil {
			t.Errorf("SOCK=%q should error, got nil", v)
		}
	}
}

func TestLoadConfig_RejectsInvalidLogLevel(t *testing.T) {
	for _, v := range []string{"warning", "INFOO", "trace"} {
		_, err := LoadConfig(mapEnv(map[string]string{
			"RCP_NS_PROXY_ALLOW_CIDR": "192.168.0.0/16",
			"RCP_NS_PROXY_LOG_LEVEL":  v,
		}))
		if err == nil {
			t.Errorf("LOG_LEVEL=%q should error, got nil", v)
		}
	}
}

func TestLoadConfig_LogLevelCaseInsensitive(t *testing.T) {
	cfg, err := LoadConfig(mapEnv(map[string]string{
		"RCP_NS_PROXY_ALLOW_CIDR": "192.168.0.0/16",
		"RCP_NS_PROXY_LOG_LEVEL":  "DEBUG",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}
