package main

import (
	"errors"
	"log"
	"os"
	"strings"

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

	dbDriver := os.Getenv("DB_DRIVER")
	if dbDriver == "" {
		dbDriver = "sqlite3"
	}
	dbDSN := os.Getenv("DB_DSN")
	if dbDSN == "" {
		dbDSN = "file:rcp.db?cache=shared&_pragma=foreign_keys(1)"
	}
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
	notifySecret := []byte(strings.TrimSpace(os.Getenv("RCP_SSH_GW_NOTIFY_SECRET")))

	myApp, err := server.NewApp(server.AppDeps{
		Provider:         provider,
		EntClient:        db,
		OAuthConfig:      oauth,
		OpenStackProject: os.Getenv("OS_PROJECT_ID"),
		JWTSecret:        jwtSecret,
		SSHGatewaySock:   notifySock,
		SSHGatewaySecret: notifySecret,
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
