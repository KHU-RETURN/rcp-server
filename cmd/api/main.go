package main

import (
	"errors"
	"log"
	"os"

	"github.com/KHU-RETURN/rcp-server/internal/config"
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

	cfg := config.Load()

	provider, err := openstack.NewProviderClient(openstack.ProviderConfig{
		AuthURL:     cfg.OpenStack.AuthURL,
		Username:    cfg.OpenStack.Username,
		Password:    cfg.OpenStack.Password,
		ProjectName: cfg.OpenStack.ProjectName,
		DomainName:  cfg.OpenStack.DomainName,
		CFClientID:  cfg.OpenStack.CFClientID,
		CFSecret:    cfg.OpenStack.CFSecret,
	})
	if err != nil {
		log.Fatalf("OpenStack 인증 실패: %v", err)
	}

	db, err := database.NewSQLiteConnection()
	if err != nil {
		log.Fatalf("DB 연결 실패: %v", err)
	}

	oauth, err := google.NewGoogleConfig(google.OAuthConfig{
		ClientID:     cfg.Google.ClientID,
		ClientSecret: cfg.Google.ClientSecret,
		RedirectURL:  cfg.Google.RedirectURL,
	})
	if err != nil {
		log.Fatalf("google oauth 연결 실패: %v", err)
	}

	myApp, err := server.NewApp(provider, db, oauth, cfg.OpenStack.ProjectID, cfg.JWTSecret, cfg.SSH)
	if err != nil {
		log.Fatalf("App 초기화 실패: %v", err)
	}

	if myApp.SSH != nil {
		go func() {
			if err := myApp.SSH.ListenAndServe(); err != nil {
				log.Printf("SSH 서버 종료: %v", err)
			}
		}()
	}

	r := server.NewRouter(myApp)

	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("HTTP 서버 시작 실패: %v", err)
	}
}
