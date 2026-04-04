package main

import (
	"errors"
	"log"
	"os"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/google"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/KHU-RETURN/rcp-server/internal/server"

	"github.com/joho/godotenv"
)

//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init --generalInfo main.go --dir .,../../internal/api,../../internal/domain/access,../../internal/domain/compute --output ../../docs/generated --outputTypes yaml --parseInternal

// @title RCP Server API
// @version 0.1.0
// @description Local development reference for the RCP server.
// @BasePath /
// @schemes http
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

	db, err := database.NewSQLiteConnection()
	if err != nil {
		log.Fatalf("DB 연결 실패: %v", err)
	}

	oauth, err := google.NewGoogleConfig(google.OAuthConfig{
		ClientID:     os.Getenv("GG_OAUTH_CLIENT"),
		ClientSecret: os.Getenv("GG_OAUTH_SECRET"),
		RedirectURL:  os.Getenv("GG_REDIRECT_URL"),
	})
	if err != nil {
		log.Fatalf("google oauth 연결 실패: %v", err)
	}

	myApp, err := server.NewApp(provider, db, oauth, os.Getenv("OS_PROJECT_ID"), jwtSecret)
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
