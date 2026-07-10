package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/KHU-RETURN/rcp-server/internal/domain/storage"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/google"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/KHU-RETURN/rcp-server/internal/server"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf(".env 로드 실패: %v", err)
	}

	// 환경변수는 main에서만 읽습니다.
	jwtSecret := os.Getenv("RCP_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-low-security-key-for-dev" // #nosec G101
	}

	provider, err := openstack.NewProviderClient(openstack.ProviderConfig{
		AuthURL:     os.Getenv("OS_AUTH_URL"),
		Username:    os.Getenv("OS_USERNAME"),
		Password:    os.Getenv("OS_PASSWORD"),
		ProjectName: os.Getenv("OS_PROJECT_NAME"),
		DomainName:  os.Getenv("OS_USER_DOMAIN_NAME"),
		CFClientID:  os.Getenv("CF_ACCESS_CLIENT_ID"),
		CFSecret:    os.Getenv("CF_ACCESS_CLIENT_SECRET"),
	})
	if err != nil {
		log.Fatalf("OpenStack 인증 실패: %v", err)
	}

	dbDriver, dbDSN := resolveDBConfig(os.Getenv)
	db, err := database.NewEntClient(database.Config{
		Driver: dbDriver,
		DSN:    dbDSN,
	})
	if err != nil {
		log.Fatalf("DB 연결 실패: %v", err)
	}
	defer func() { _ = db.Close() }()

	oauth, err := google.NewGoogleConfig(google.OAuthConfig{
		ClientID:     os.Getenv("GG_OAUTH_CLIENT"),
		ClientSecret: os.Getenv("GG_OAUTH_SECRET"),
		RedirectURL:  os.Getenv("GG_REDIRECT_URL"),
	})
	if err != nil {
		log.Fatalf("google oauth 연결 실패: %v", err)
	}

	usageLimits, err := loadUserUsageLimits()
	if err != nil {
		log.Fatal(err)
	}

	storageLimits, err := loadUserStorageLimits()
	if err != nil {
		log.Fatal(err)
	}

	notifySock := strings.TrimSpace(os.Getenv("RCP_SSH_GW_NOTIFY_SOCK"))
	if notifySock == "" {
		notifySock = "/run/rcp/ssh-gateway-notify.sock"
	}
	notifySecret := []byte(strings.TrimSpace(os.Getenv("RCP_SSH_GW_NOTIFY_SECRET")))
	nsProxySock := strings.TrimSpace(os.Getenv("RCP_NS_PROXY_SOCK"))
	if nsProxySock == "" {
		nsProxySock = "/run/rcp/ns-proxy.sock"
	}
	httpProxyAddress := resolveAppGatewayAddress(os.Getenv)

	frontendBaseURL := resolveFrontendBaseURL(os.Getenv)
	if frontendBaseURL == "" {
		log.Fatalf("RCP_FRONTEND_BASE_URL or FRONTEND_URL: required (e.g. https://rcp.return.dev)")
	}

	myApp, err := server.NewApp(server.AppDeps{
		Provider:         provider,
		EntClient:        db,
		OAuthConfig:      oauth,
		OpenStackProject: os.Getenv("OS_PROJECT_ID"),
		DefaultNetworkID: os.Getenv("RCP_DEFAULT_NETWORK_ID"),
		JWTSecret:        jwtSecret,
		SSHGatewaySock:   notifySock,
		SSHGatewaySecret: notifySecret,
		NSProxySock:      nsProxySock,
		HTTPProxyAddress: httpProxyAddress,
		FrontendBaseURL:  frontendBaseURL,
		UsageLimits:      usageLimits,
		StorageLimits:    storageLimits,
	})
	if err != nil {
		log.Fatalf("App 초기화 실패: %v", err)
	}

	r := server.NewRouter(myApp)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("HTTP 서버 시작 실패: %v", err)
	}
}

func loadUserUsageLimits() (compute.UserUsageLimits, error) {
	instances, err := parseNonNegativeEnv("RCP_MAX_INSTANCES_PER_USER")
	if err != nil {
		return compute.UserUsageLimits{}, err
	}
	vcpus, err := parseNonNegativeEnv("RCP_MAX_VCPUS_PER_USER")
	if err != nil {
		return compute.UserUsageLimits{}, err
	}
	ramMB, err := parseNonNegativeEnv("RCP_MAX_RAM_MB_PER_USER")
	if err != nil {
		return compute.UserUsageLimits{}, err
	}
	diskGB, err := parseNonNegativeEnv("RCP_MAX_DISK_GB_PER_USER")
	if err != nil {
		return compute.UserUsageLimits{}, err
	}
	return compute.UserUsageLimits{
		Instances: instances,
		VCPUs:     vcpus,
		RAMMB:     ramMB,
		DiskGB:    diskGB,
	}, nil
}

func loadUserStorageLimits() (storage.UserStorageLimits, error) {
	containers, err := parseNonNegativeEnv("RCP_MAX_CONTAINERS_PER_USER")
	if err != nil {
		return storage.UserStorageLimits{}, err
	}
	storageGB, err := parseNonNegativeEnv("RCP_MAX_STORAGE_GB_PER_USER")
	if err != nil {
		return storage.UserStorageLimits{}, err
	}
	return storage.UserStorageLimits{Containers: containers, StorageGB: storageGB}, nil
}

func resolveFrontendBaseURL(getenv func(string) string) string {
	if url := strings.TrimSpace(getenv("FRONTEND_URL")); url != "" {
		return strings.TrimRight(url, "/")
	}
	if url := strings.TrimSpace(getenv("RCP_FRONTEND_BASE_URL")); url != "" {
		return strings.TrimRight(url, "/")
	}
	return ""
}

func resolveAppGatewayAddress(getenv func(string) string) string {
	port := strings.TrimSpace(getenv("APP_GATEWAY_PORT"))
	if port == "" {
		port = "18080"
	}
	if strings.HasPrefix(port, ":") {
		return "127.0.0.1" + port
	}
	if strings.Contains(port, ":") {
		return port
	}
	return "127.0.0.1:" + port
}

func resolveDBConfig(getenv func(string) string) (string, string) {
	driver := strings.TrimSpace(getenv("DB_DRIVER"))
	if driver == "" {
		driver = "sqlite3"
	}
	dsn := strings.TrimSpace(getenv("DB_DSN"))
	if dsn == "" {
		dsn = "file:rcp.db?cache=shared&_pragma=foreign_keys(1)"
	}
	return driver, dsn
}

func parseNonNegativeEnv(name string) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be greater than or equal to 0", name)
	}
	return value, nil
}
