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

// BuildLoginURL은 Google OAuth 승인 페이지 URL을 만듭니다. stateOverride가
// 비어 있으면 CSRF-safe 랜덤 state를 생성하고, 비어 있지 않으면 호출자가
// 넘긴 state(예: ssh-gateway가 검증할 ssh:<nonce>:<code>)를 그대로 사용합니다.
func (s *Service) BuildLoginURL(stateOverride string) string {
	state := stateOverride
	if state == "" {
		state = s.generateState(oauthStateByteLen)
	}
	return s.OauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// GetGoogleLoginURL은 사용자를 리다이렉트시킬 구글 승인 페이지 URL과, 호출자가
// double-submit 쿠키로 저장해 콜백에서 검증해야 할 state nonce를 함께 반환합니다.
func (s *Service) GetGoogleLoginURL(redirectOrigin string) (loginURL, nonce string) {
	nonce = s.generateState(oauthStateByteLen)
	state := s.encodeOAuthState(nonce, redirectOrigin)
	return s.OauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline), nonce
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

	rawIDToken, _ := token.Extra(googleIDTokenKey).(string)
	payload, err := idtoken.Validate(ctx, rawIDToken, s.OauthConfig.ClientID)
	if err != nil {
		return nil, fmt.Errorf("invalid id_token: %w", err)
	}

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

	tokens, err := s.TokenService.GenerateAuthTokens(id.email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate service tokens: %w", err)
	}

	jti := tokens.RefreshJTI
	user := &User{
		Email:             id.email,
		Name:              id.name,
		GoogleID:          id.subject,
		AccessToken:       tokens.AccessToken,
		RefreshToken:      tokens.RefreshToken,
		Expiry:            tokens.AccessExpiry,
		CurrentRefreshJTI: &jti,
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

// generateState는 CSRF 공격 방지를 위한 임의의 문자열을 생성합니다.
func (s *Service) generateState(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random state: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Service) encodeOAuthState(nonce, redirectOrigin string) string {
	state := oauthState{
		Nonce:          nonce,
		RedirectOrigin: redirectOrigin,
	}
	b, err := json.Marshal(state)
	if err != nil {
		panic(fmt.Sprintf("failed to generate oauth state: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
