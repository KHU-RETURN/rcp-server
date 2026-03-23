package main

import (
	"errors"
	"log"
	"os"

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
