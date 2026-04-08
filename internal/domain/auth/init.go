package auth

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"golang.org/x/oauth2"
)

func Init(client *ent.Client, oauthConfig *oauth2.Config, jwtSecret string) (*Handler, error) {
	repo := NewRepository(client)
	tokenSvc := NewTokenService(jwtSecret)
	svc := NewService(repo, oauthConfig, tokenSvc)
	return NewHandler(svc), nil
}
