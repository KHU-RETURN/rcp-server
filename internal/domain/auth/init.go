package auth

import (
	"database/sql"

	"golang.org/x/oauth2"
)

func Init(db *sql.DB, oauthConfig *oauth2.Config, jwtSecret string) (*Handler, error) {
	repo, err := NewRepository(db)
	if err != nil {
		return nil, err
	}
	tokenSvc := NewTokenService(jwtSecret)
	svc := NewService(repo, oauthConfig, tokenSvc)
	return NewHandler(svc), nil
}
