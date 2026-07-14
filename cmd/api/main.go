package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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
	})
	if err != nil {
		log.Fatalf("App 초기화 실패: %v", err)
	}

	r := server.NewRouter(myApp)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// gin의 기본 서버는 타임아웃이 없어 Slowloris에 취약하다. ReadTimeout/WriteTimeout은
	// 콘솔 websocket 연결이 장시간 유지돼야 해서 일부러 설정하지 않는다.
	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("HTTP 서버 시작 실패: %v", err)
	}
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
		dsn = "file:rcp.db?cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	}
	return driver, dsn
}
