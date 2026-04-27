package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	SockPath      string
	AllowList     *Allowlist
	MaxConns      int
	DialTimeout   time.Duration
	ShutdownGrace time.Duration
	LogLevel      string
}

// LoadConfig reads configuration from a getenv function (typically os.Getenv).
// The getenv parameter makes the function trivially testable.
func LoadConfig(getenv func(string) string) (*Config, error) {
	al, err := ParseCIDRs(getenv("RCP_NS_PROXY_ALLOW_CIDR"))
	if err != nil {
		return nil, fmt.Errorf("RCP_NS_PROXY_ALLOW_CIDR: %w", err)
	}
	maxConns, err := envInt(getenv, "RCP_NS_PROXY_MAX_CONNS", 1024)
	if err != nil {
		return nil, err
	}
	if maxConns <= 0 {
		return nil, fmt.Errorf("RCP_NS_PROXY_MAX_CONNS: must be positive, got %d", maxConns)
	}
	dialTimeout, err := envPositiveDuration(getenv, "RCP_NS_PROXY_DIAL_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	grace, err := envPositiveDuration(getenv, "RCP_NS_PROXY_SHUTDOWN_GRACE", 30*time.Second)
	if err != nil {
		return nil, err
	}
	sockPath, err := envSockPath(getenv, "RCP_NS_PROXY_SOCK", "/run/rcp/ns-proxy.sock")
	if err != nil {
		return nil, err
	}
	logLevel, err := envLogLevel(getenv, "RCP_NS_PROXY_LOG_LEVEL", "info")
	if err != nil {
		return nil, err
	}
	return &Config{
		SockPath:      sockPath,
		AllowList:     al,
		MaxConns:      maxConns,
		DialTimeout:   dialTimeout,
		ShutdownGrace: grace,
		LogLevel:      logLevel,
	}, nil
}

func envString(get func(string) string, key, def string) string {
	if v := get(key); v != "" {
		return v
	}
	return def
}

func envInt(get func(string) string, key string, def int) (int, error) {
	v := get(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func envDuration(get func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := get(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func envPositiveDuration(get func(string) string, key string, def time.Duration) (time.Duration, error) {
	d, err := envDuration(get, key, def)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: must be > 0, got %v", key, d)
	}
	return d, nil
}

func envSockPath(get func(string) string, key, def string) (string, error) {
	raw := get(key)
	v := strings.TrimSpace(raw)
	if raw != "" && v == "" {
		// env var was set but contained only whitespace
		return "", fmt.Errorf("%s: must be an absolute path, got %q", key, raw)
	}
	if v == "" {
		v = def
	}
	if !filepath.IsAbs(v) {
		return "", fmt.Errorf("%s: must be an absolute path, got %q", key, v)
	}
	return v, nil
}

func envLogLevel(get func(string) string, key, def string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(get(key)))
	if v == "" {
		return def, nil
	}
	switch v {
	case "debug", "info", "warn", "error":
		return v, nil
	default:
		return "", fmt.Errorf("%s: invalid log level %q (allowed: debug, info, warn, error)", key, v)
	}
}
