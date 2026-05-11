package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// userRepository는 유저 데이터 접근 인터페이스입니다.
// 구현체는 repository.go의 Repository입니다.
type userRepository interface {
	UpsertUser(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
}

// Service는 인증 비즈니스 로직을 담당합니다.
type Service struct {
	repo         userRepository
	OauthConfig  *oauth2.Config
	TokenService *TokenService
}

func NewService(repo userRepository, config *oauth2.Config, svc *TokenService) *Service {
	return &Service{
		repo:         repo,
		OauthConfig:  config,
		TokenService: svc,
	}
}

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
		return nil, errors.New("email claim not found in id_token")
	}
	if !strings.HasSuffix(email, "@khu.ac.kr") {
		return nil, errors.New("경희대학교 계정(@khu.ac.kr)으로만 로그인할 수 있습니다")
	}
	name, _ := payload.Claims["name"].(string)
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
	if id.name == "" {
		return nil, errors.New("name claim not found in id_token")
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
