package main

import (
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/KHU-RETURN/rcp-server/internal/server"
	"github.com/joho/godotenv"
	"log"
	"os"
)

func main() {
	godotenv.Load()

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
	r.Run(":" + port)
}
