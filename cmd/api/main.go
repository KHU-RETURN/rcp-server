package main

import (
	"database/sql"
	"errors"
	"log"
	"os"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/google"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/KHU-RETURN/rcp-server/internal/server"

	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf(".env 로드 실패: %v", err)
	}

	provider, err := openstack.NewProviderClient()
	if err != nil {
		log.Fatalf("OpenStack 인증 실패: %v", err)
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatalf("DB 연결 실패: %v", err)
	}
	oauth, err := google.NewGoogleConfig()
	if err != nil {
		log.Fatalf("google oauth 연결 실패: %v", err)
	}

	myApp := server.NewApp(provider, db, oauth)
	r := server.NewRouter(myApp)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("HTTP 서버 시작 실패: %v", err)
	}
}
