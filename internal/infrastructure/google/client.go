package google

import (
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func NewGoogleConfig() (*oauth2.Config, error) {
	clientID := os.Getenv("GG_OAUTH_CLIENT")
	clientSecret := os.Getenv("GG_OAUTH_SECRET")

	// 1. 필수 환경 변수 체크
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("missing required environment variables: GG_OAUTH_CLIENT or GG_OAUTH_SECRET")
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "http://localhost:8080/api/v1/auth/google/callback", // 하드코딩 대신 환경 변수 추천
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
			"openid",
		},
		Endpoint: google.Endpoint,
	}

	// 2. 추가 검증: RedirectURL이 비어있는지 확인
	if config.RedirectURL == "" {
		// 기본값 설정 혹은 에러 처리
		config.RedirectURL = "http://localhost:8080/api/v1/auth/google/callback"
	}

	return config, nil
}
