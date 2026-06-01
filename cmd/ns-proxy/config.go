package main

import (
	"fmt"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/utils"
)

type Config struct {
	SockPath      string
	AllowList     *Allowlist
	MaxConns      int
	DialTimeout   time.Duration
	ShutdownGrace time.Duration
	LogLevel      string
}

// LoadConfig는 getenv 함수(보통 os.Getenv)에서 설정을 읽는다.
// getenv를 파라미터로 받아 테스트가 단순해짐.
func LoadConfig(getenv func(string) string) (*Config, error) {
	al, err := ParseCIDRs(getenv("RCP_NS_PROXY_ALLOW_CIDR"))
	if err != nil {
		return nil, fmt.Errorf("RCP_NS_PROXY_ALLOW_CIDR: %w", err)
	}
	maxConns, err := utils.EnvInt(getenv, "RCP_NS_PROXY_MAX_CONNS", 1024)
	if err != nil {
		return nil, err
	}
	if maxConns <= 0 {
		return nil, fmt.Errorf("RCP_NS_PROXY_MAX_CONNS: must be positive, got %d", maxConns)
	}
	dialTimeout, err := utils.EnvPositiveDuration(getenv, "RCP_NS_PROXY_DIAL_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	grace, err := utils.EnvPositiveDuration(getenv, "RCP_NS_PROXY_SHUTDOWN_GRACE", 30*time.Second)
	if err != nil {
		return nil, err
	}
	sockPath, err := utils.EnvSockPath(getenv, "RCP_NS_PROXY_SOCK", "/run/rcp/ns-proxy.sock")
	if err != nil {
		return nil, err
	}
	logLevel, err := utils.EnvLogLevel(getenv, "RCP_NS_PROXY_LOG_LEVEL", "info")
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
