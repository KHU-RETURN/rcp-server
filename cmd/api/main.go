package main

import (
	"log"
	"os"
	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/KHU-RETURN/rcp-server/internal/server"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	provider, err := openstack.NewProviderClient()
	if err != nil {
		log.Fatalf("OpenStack 인증 실패: %v", err)
	}

	repo := compute.NewRepository(provider)
	svc := compute.NewService(repo)
	handler := compute.NewHandler(svc)

	r := server.NewRouter(handler)
	
	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	r.Run(":" + port)
}
