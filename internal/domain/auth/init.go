package auth

import (
	"golang.org/x/oauth2"

	"github.com/KHU-RETURN/rcp-server/ent"
)

func Init(db *ent.Client, oauthConfig *oauth2.Config, jwtSecret string) (*Handler, error) {
	repo := NewRepository(db)
	tokenSvc := NewTokenService(jwtSecret)
	svc := NewService(repo, oauthConfig, tokenSvc)
	return NewHandler(svc), nil
}
