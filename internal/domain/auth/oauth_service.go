package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// BuildLoginURL은 Google OAuth 승인 페이지 URL을 만듭니다. stateOverride가
// 비어 있으면 CSRF-safe 랜덤 state를 생성하고, 비어 있지 않으면 호출자가
// 넘긴 state(예: ssh-gateway가 발급한 ssh:<nonce>)를 그대로 사용합니다.
func (s *Service) BuildLoginURL(stateOverride string) string {
	state := stateOverride
	if state == "" {
		state = s.generateState(16)
	}
	return s.OauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// verifiedGoogleIdentity는 id_token 검증으로 확인된 사용자 정보입니다.
// googleToken은 후속 단계(예: User 저장)에서 Google API 호출에 사용됩니다.
type verifiedGoogleIdentity struct {
	email       string
	name        string
	subject     string
	googleToken *oauth2.Token
}

// verifyGoogleCode는 OAuth code 교환 + id_token 검증 + @khu.ac.kr 도메인
// 강제까지 한 번에 처리하고 검증된 신원을 돌려줍니다.
func (s *Service) verifyGoogleCode(ctx context.Context, code string) (*verifiedGoogleIdentity, error) {
	token, err := s.OauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	payload, err := idtoken.Validate(ctx, rawIDToken, s.OauthConfig.ClientID)
	if err != nil {
		return nil, fmt.Errorf("id_token invalid: %w", err)
	}
	email, ok := payload.Claims["email"].(string)
	if !ok || email == "" {
		return nil, ErrEmailClaimNotFound
	}

	name, ok := payload.Claims["name"].(string)
	if !ok || name == "" {
		return nil, ErrNameClaimNotFound
	}

	if !strings.HasSuffix(email, "@khu.ac.kr") {
		return nil, ErrUnsupportedEmailDomain
	}
	return &verifiedGoogleIdentity{
		email:       email,
		name:        name,
		subject:     payload.Subject,
		googleToken: token,
	}, nil
}

// ProcessGoogleCallback은 일반 로그인 콜백을 처리합니다. 신원 검증 후
// 서비스 JWT를 발급하고 User를 DB에 upsert합니다.
func (s *Service) ProcessGoogleCallback(ctx context.Context, code string) (*User, error) {
	id, err := s.verifyGoogleCode(ctx, code)
	if err != nil {
		return nil, err
	}
	accessToken, refreshToken, expiry, err := s.TokenService.GenerateAuthTokens(id.email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate service tokens: %w", err)
	}
	user := &User{
		Email:        id.email,
		Name:         id.name,
		GoogleID:     id.subject,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
		GoogleAuth: &GoogleInfo{
			AccessToken:  id.googleToken.AccessToken,
			RefreshToken: id.googleToken.RefreshToken,
			Expiry:       id.googleToken.Expiry,
		},
	}
	if err := s.repo.UpsertUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}
	return user, nil
}

// VerifyGoogleCode는 신원만 필요한 흐름(예: ssh-gateway 세션 해소)에서
// 서비스 토큰 발급 없이 검증된 email만 돌려줍니다.
func (s *Service) VerifyGoogleCode(ctx context.Context, code string) (string, error) {
	id, err := s.verifyGoogleCode(ctx, code)
	if err != nil {
		return "", err
	}
	return id.email, nil
}

func (s *Service) generateState(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random state: %v", err))
	}
	return base64.URLEncoding.EncodeToString(b)
}
