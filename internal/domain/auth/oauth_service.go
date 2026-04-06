package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"strings"
)

// Service는 인증 비즈니스 로직을 담당합니다.
type Service struct {
	Repo         UserRepository
	OauthConfig  *oauth2.Config
	TokenService *TokenService // JWT 발급을 위해 주입받음
}

// NewService는 새로운 서비스를 생성합니다.
func NewService(repo UserRepository, config *oauth2.Config, svc *TokenService) *Service {
	return &Service{
		Repo:         repo,
		OauthConfig:  config,
		TokenService: svc,
	}
}

// GetGoogleLoginURL은 사용자를 리다이렉트시킬 구글 승인 페이지 URL을 생성합니다.
func (s *Service) GetGoogleLoginURL() string {
	// 실제 운영 환경에서는 state를 세션에 저장하고 콜백에서 검증해야 보안상 안전합니다.
	state := s.generateState(16)
	return s.OauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline) // AccessTypeOffline은 Refresh Token을 받기 위함
}

// ProcessGoogleCallback은 구글로부터 받은 code를 토큰으로 교환하고 유저 정보를 처리합니다.

func (s *Service) ProcessGoogleCallback(ctx context.Context, code string) (*User, error) {
	// 1. 토큰 교환 로직 (기존 코드)
	token, err := s.OauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// 2. ID 토큰 검증 및 페이로드 추출 (기존 코드)
	rawIDToken, _ := token.Extra("id_token").(string)
	payload, err := idtoken.Validate(ctx, rawIDToken, s.OauthConfig.ClientID)
	if err != nil {
		return nil, fmt.Errorf("no id_token in token response")
	}

	// 🔒 2.5 이메일 도메인 검증 로직 추가
	email, ok := payload.Claims["email"].(string)
	if !ok || email == "" {
		return nil, errors.New("email claim not found in id_token")
	}

	name, ok := payload.Claims["name"].(string)
	if !ok || name == "" {
		return nil, errors.New("name claim not found in id_token")
	}
	if !strings.HasSuffix(email, "@khu.ac.kr") {
		return nil, errors.New("경희대학교 계정(@khu.ac.kr)으로만 로그인할 수 있습니다")
	}
	// 3. 우리 서비스 토큰 발급 (TokenService 활용)
	accessToken, refreshToken, expiry, err := s.TokenService.GenerateAuthTokens(email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate service tokens: %w", err)
	}

	// 3. User 객체 생성 및 DB 저장 (기존 코드)
	user := &User{
		Email:        email,
		Name:         name,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
		// 👉 Google 전용 토큰은 여기 넣어야 함
		GoogleAuth: &GoogleInfo{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			Expiry:       token.Expiry,
		},
	}

	if err := s.Repo.UpsertUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	return user, nil
}

// generateState는 CSRF 공격 방지를 위한 임의의 문자열을 생성합니다.
func (s *Service) generateState(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random state: %v", err))
	}
	return base64.URLEncoding.EncodeToString(b)
}
