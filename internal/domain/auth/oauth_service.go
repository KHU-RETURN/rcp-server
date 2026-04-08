package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// authRepository는 유저/세션 데이터 접근 인터페이스입니다.
// 구현체는 repository.go의 Repository입니다.
type authRepository interface {
	UpsertUser(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	CreateSession(ctx context.Context, userID uuid.UUID, session *Session) error
}

// Service는 인증 비즈니스 로직을 담당합니다.
type Service struct {
	repo         authRepository
	OauthConfig  *oauth2.Config
	TokenService *TokenService // JWT 발급을 위해 주입받음
}

// NewService는 새로운 서비스를 생성합니다.
func NewService(repo authRepository, config *oauth2.Config, svc *TokenService) *Service {
	return &Service{
		repo:         repo,
		OauthConfig:  config,
		TokenService: svc,
	}
}

// GetGoogleLoginURL은 사용자를 리다이렉트시킬 구글 승인 페이지 URL을 생성합니다.
func (s *Service) GetGoogleLoginURL() string {
	state := s.generateState(16)
	return s.OauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ProcessGoogleCallback은 구글로부터 받은 code를 토큰으로 교환하고 유저 정보를 처리합니다.
func (s *Service) ProcessGoogleCallback(ctx context.Context, code string) (*User, error) {
	// 1. Google 토큰 교환
	token, err := s.OauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// 2. ID 토큰 검증 및 클레임 추출
	rawIDToken, _ := token.Extra("id_token").(string)
	payload, err := idtoken.Validate(ctx, rawIDToken, s.OauthConfig.ClientID)
	if err != nil {
		return nil, fmt.Errorf("no id_token in token response")
	}

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

	// 3. 서비스 JWT 발급
	accessToken, refreshToken, expiry, err := s.TokenService.GenerateAuthTokens(email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate service tokens: %w", err)
	}

	// 4. User upsert (프로필 정보만 저장) → user.ID 획득
	user := &User{
		Email: email,
		Name:  name,
	}
	if err := s.repo.UpsertUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	// 5. Session 생성 (JWT + Google 토큰 저장)
	var providerRefresh *string
	if token.RefreshToken != "" {
		providerRefresh = &token.RefreshToken
	}
	var providerExpiry *time.Time
	if !token.Expiry.IsZero() {
		t := token.Expiry
		providerExpiry = &t
	}
	sess := &Session{
		AccessToken:     accessToken,
		RefreshToken:    refreshToken,
		Expiry:          expiry,
		Provider:        "GOOGLE",
		ProviderToken:   token.AccessToken,
		ProviderRefresh: providerRefresh,
		ProviderExpiry:  providerExpiry,
	}
	if err := s.repo.CreateSession(ctx, user.ID, sess); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// 6. 응답용 transient 필드 설정
	user.AccessToken = accessToken
	user.RefreshToken = refreshToken
	user.Expiry = expiry

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
