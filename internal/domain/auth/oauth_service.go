package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

const (
	// 학내 계정만 로그인 허용 (이메일 suffix 매칭).
	allowedEmailDomain = "@khu.ac.kr"

	// Google ID Token claim 키.
	googleIDTokenKey = "id_token"
	claimKeyEmail    = "email"
	claimKeyName     = "name"

	// OAuth state 파라미터의 raw 바이트 길이 (base64 인코딩 전).
	oauthStateByteLen = 16
)

type oauthState struct {
	Nonce          string `json:"nonce"`
	RedirectOrigin string `json:"redirect_origin,omitempty"`
}

// GetGoogleLoginURL은 사용자를 리다이렉트시킬 구글 승인 페이지 URL을 생성합니다.
func (s *Service) GetGoogleLoginURL(redirectOrigin string) string {
	// 실제 운영 환경에서는 state를 세션에 저장하고 콜백에서 검증해야 보안상 안전합니다.
	state := s.generateOAuthState(redirectOrigin)
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
	rawIDToken, _ := token.Extra(googleIDTokenKey).(string)
	payload, err := idtoken.Validate(ctx, rawIDToken, s.OauthConfig.ClientID)
	if err != nil {
		return nil, fmt.Errorf("invalid id_token: %w", err)
	}

	// 🔒 2.5 이메일 도메인 검증 로직 추가
	email, ok := payload.Claims[claimKeyEmail].(string)
	if !ok || email == "" {
		return nil, ErrEmailClaimNotFound
	}

	name, ok := payload.Claims[claimKeyName].(string)
	if !ok || name == "" {
		return nil, ErrNameClaimNotFound
	}
	if !strings.HasSuffix(email, allowedEmailDomain) {
		return nil, ErrUnsupportedEmailDomain
	}
	// 3. 우리 서비스 토큰 발급 (TokenService 활용)
	tokens, err := s.TokenService.GenerateAuthTokens(email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate service tokens: %w", err)
	}

	// 4. User 객체 생성 및 DB 저장 (refresh jti를 함께 저장하여 서버측 회전/무효화 가능)
	jti := tokens.RefreshJTI
	user := &User{
		Email:             email,
		Name:              name,
		GoogleID:          payload.Subject,
		AccessToken:       tokens.AccessToken,
		RefreshToken:      tokens.RefreshToken,
		Expiry:            tokens.AccessExpiry,
		CurrentRefreshJTI: &jti,
		GoogleAuth: &GoogleInfo{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			Expiry:       token.Expiry,
		},
	}

	if err := s.repo.UpsertUser(ctx, user); err != nil {
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

func (s *Service) generateOAuthState(redirectOrigin string) string {
	state := oauthState{
		Nonce:          s.generateState(oauthStateByteLen),
		RedirectOrigin: redirectOrigin,
	}
	b, err := json.Marshal(state)
	if err != nil {
		panic(fmt.Sprintf("failed to generate oauth state: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
