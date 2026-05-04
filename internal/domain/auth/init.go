package auth

import (
	"github.com/KHU-RETURN/rcp-server/ent"
	"golang.org/x/oauth2"
)

func Init(db *ent.Client, oauthConfig *oauth2.Config, jwtSecret string, ssh sshCallbackHandler) (*Handler, error) {
	repo := NewRepository(db)
	tokenSvc := NewTokenService(jwtSecret)
	svc := NewService(repo, oauthConfig, tokenSvc)
	return NewHandler(svc, ssh), nil
}
