package auth

import (
	"database/sql"
	"os"

	"golang.org/x/oauth2"
)

func Init(db *sql.DB, oauthConfig *oauth2.Config) (*Handler, error) {
	repo, err := NewRepository(db)
	if err != nil {
		return nil, err
	}
	secret := os.Getenv("RCP_JWT_SECRET")
	if secret == "" {
		secret = "default-low-security-key-for-dev" // #nosec G101
	}
	tokenSvc := NewTokenService(secret)
	svc := NewService(repo, oauthConfig, tokenSvc)
	return NewHandler(svc), nil
}
