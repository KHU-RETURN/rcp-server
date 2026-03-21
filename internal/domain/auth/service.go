package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
    "errors"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// Service는 인증 비즈니스 로직을 담당합니다.
type Service struct {
	Repo        UserRepository
	OauthConfig *oauth2.Config
}

// NewService는 새로운 서비스를 생성합니다.
func NewService(repo UserRepository, config *oauth2.Config) *Service {
	return &Service{
		Repo:        repo,
		OauthConfig: config,
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
    email := payload.Claims["email"].(string)
    if !strings.HasSuffix(email, "@khu.ac.kr") {
        return nil, errors.New("경희대학교 계정(@khu.ac.kr)으로만 로그인할 수 있습니다")
    }

    // 3. User 객체 생성 및 DB 저장 (기존 코드)
    user := &User{
        Email:        email,
        Name:         payload.Claims["name"].(string),
        AccessToken:  token.AccessToken,
        RefreshToken: token.RefreshToken,
        Expiry:       token.Expiry,
    }

    if err := s.Repo.UpsertUser(ctx, user); err != nil {
        return nil, fmt.Errorf("failed to save user: %w", err)
    }

    return user, nil
}

// generateState는 CSRF 공격 방지를 위한 임의의 문자열을 생성합니다.
func (s *Service) generateState(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
