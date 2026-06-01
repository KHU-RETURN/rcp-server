package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/utils"
)

type Config struct {
	Listen           string
	NsProxySock      string
	FixedNetworkName string
	DBDriver         string
	DBDSN            string
	ReadTimeout      time.Duration
	ShutdownTimeout  time.Duration
	LogLevel         string
}

func LoadConfig(getenv func(string) string) (*Config, error) {
	port := strings.TrimSpace(getenv("APP_GATEWAY_PORT"))
	if port == "" {
		port = "18080"
	}
	listen := ":" + strings.TrimPrefix(port, ":")
	if strings.Contains(port, ":") {
		listen = port
	}

	nsProxySock, err := utils.EnvSockPath(getenv, "RCP_NS_PROXY_SOCK", "/run/rcp/ns-proxy.sock")
	if err != nil {
		return nil, err
	}

	dbDriver := strings.TrimSpace(getenv("DB_DRIVER"))
	if dbDriver == "" {
		dbDriver = "sqlite3"
	}
	dbDSN := strings.TrimSpace(getenv("DB_DSN"))
	if dbDSN == "" {
		dbDSN = "file:rcp.db?mode=ro&cache=shared&_pragma=foreign_keys(1)"
	}
	if err := validateGatewayDBDSN(dbDriver, dbDSN); err != nil {
		return nil, err
	}

	readTimeout, err := utils.EnvPositiveDuration(getenv, "RCP_APP_GW_READ_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	shutdownTimeout, err := utils.EnvPositiveDuration(getenv, "RCP_APP_GW_SHUTDOWN_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	logLevel, err := utils.EnvLogLevel(getenv, "RCP_APP_GW_LOG_LEVEL", "info")
	if err != nil {
		return nil, err
	}

	return &Config{
		Listen:           listen,
		NsProxySock:      nsProxySock,
		FixedNetworkName: strings.TrimSpace(getenv("RCP_APP_GW_FIXED_NETWORK")),
		DBDriver:         dbDriver,
		DBDSN:            dbDSN,
		ReadTimeout:      readTimeout,
		ShutdownTimeout:  shutdownTimeout,
		LogLevel:         logLevel,
	}, nil
}

func validateGatewayDBDSN(driver, dsn string) error {
	if strings.TrimSpace(driver) == "" {
		return fmt.Errorf("DB_DRIVER: required")
	}
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("DB_DSN: required")
	}
	return nil
}
