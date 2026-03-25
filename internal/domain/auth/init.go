package auth

import (
	"database/sql"
	"golang.org/x/oauth2"
	"os"
)

// Init은 auth 모듈의 의존성을 초기화합니다.
// DB와 OAuth 설정을 주입받아 Handler까지 생성합니다.
// internal/domain/auth/init.go (또는 초기화 위치)

func Init(db *sql.DB, oauthConfig *oauth2.Config) (*Handler, error) {
	repo, err := NewRepository(db)
	if err != nil {
		return nil, err // 에러 발생 시 상위(main.go 등)로 던집니다.
	}
	secret := os.Getenv("RCP_JWT_SECRET")
	if secret == "" {
		secret = "default-low-security-key-for-dev" // #nosec G101 - This is a fallback for development only
	}
	// 1. 반드시 TokenService를 먼저 생성해야 합니다!
	tokenSvc := NewTokenService(secret)

	// 2. NewService의 세 번째 인자로 tokenSvc를 넣어주세요.
	svc := NewService(repo, oauthConfig, tokenSvc)

	return NewHandler(svc), nil
}
