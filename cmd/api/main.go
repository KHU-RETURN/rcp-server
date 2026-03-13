package main

import (
	"log"
	"net/http" // 추가됨
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/joho/godotenv"

	"github.com/KHU-RETURN/rcp-server/internal/domain/compute"
)

// Cloudflare Access 헤더를 주입하기 위한 Custom RoundTripper
type cloudflareTransport struct {
	rt           http.RoundTripper
	clientID     string
	clientSecret string
}

func (t *cloudflareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("CF-Access-Client-Id", t.clientID)
	req.Header.Set("CF-Access-Client-Secret", t.clientSecret)
	return t.rt.RoundTrip(req)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env 파일을 찾을 수 없습니다. 시스템 환경 변수를 사용합니다.")
	}

	// 1. Cloudflare 신분증 준비
	cfID := os.Getenv("CF_ACCESS_CLIENT_ID")
	cfSecret := os.Getenv("CF_ACCESS_CLIENT_SECRET")

	// 2. OpenStack 인증 정보 설정
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("OS_AUTH_URL"),
		Username:         os.Getenv("OS_USERNAME"),
		Password:         os.Getenv("OS_PASSWORD"),
		TenantName:       os.Getenv("OS_PROJECT_NAME"),
		DomainName:       os.Getenv("OS_USER_DOMAIN_NAME"),
	}

	// 3. [핵심] HTTP 클라이언트에 클라우드플레어 헤더 주입
	provider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		log.Fatalf("Provider 클라이언트 생성 실패: %v", err)
	}

	// 모든 요청에 CF 헤더를 붙이도록 설정
	provider.HTTPClient = http.Client{
		Transport: &cloudflareTransport{
			rt:           http.DefaultTransport,
			clientID:     cfID,
			clientSecret: cfSecret,
		},
	}

	// 4. 인증 진행
	err = openstack.Authenticate(provider, opts)
	if err != nil {
		log.Fatalf("OpenStack 인증 실패: %v", err)
	}

	// 5. 의존성 주입 및 서버 실행 (기존과 동일)
	repo := compute.NewRepository(provider)
	svc := compute.NewService(repo)
	handler := compute.NewHandler(svc)

	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ready"})
	})

	v1 := r.Group("/api/v1")
	{
		v1.GET("/flavors", handler.GetFlavors)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 RCP Server started on :%s", port)
	r.Run(":" + port)
}
