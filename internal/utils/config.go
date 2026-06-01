package utils

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func EnvInt(get func(string) string, key string, def int) (int, error) {
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

func EnvPositiveDuration(get func(string) string, key string, def time.Duration) (time.Duration, error) {
	d, err := envDuration(get, key, def)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: must be > 0, got %v", key, d)
	}
	return d, nil
}

func EnvSockPath(get func(string) string, key, def string) (string, error) {
	raw := get(key)
	v := strings.TrimSpace(raw)
	if raw != "" && v == "" {
		// 값은 설정됐지만 공백만 있는 경우.
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

func EnvLogLevel(get func(string) string, key, def string) (string, error) {
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
