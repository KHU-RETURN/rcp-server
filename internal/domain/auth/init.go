package auth

import (
	"database/sql"

	"golang.org/x/oauth2"
)

// Init은 auth 모듈의 의존성을 초기화합니다.
// DB와 OAuth 설정을 주입받아 Handler까지 생성합니다.
func Init(db *sql.DB, oauthConfig *oauth2.Config) *Handler {
	repo := NewRepository(db)
	svc := NewService(repo, oauthConfig)
	return NewHandler(svc)
}