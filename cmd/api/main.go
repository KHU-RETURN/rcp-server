package main

import (
	"errors"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/KHU-RETURN/rcp-server/internal/server"
	"github.com/joho/godotenv"
	"log"
	"os"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf(".env 로드 실패: %v", err)
	}

	provider, err := openstack.NewProviderClient()
	if err != nil {
		log.Fatalf("OpenStack 인증 실패: %v", err)
	}

	myApp := server.NewApp(provider)
	r := server.NewRouter(myApp)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("HTTP 서버 시작 실패: %v", err)
	}
}
