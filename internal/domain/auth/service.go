package auth

import (
	"context"
	"errors"

	"golang.org/x/oauth2"
)

var (
	ErrOAuthCodeRequired      = errors.New("code is required")
	ErrEmailClaimNotFound     = errors.New("email claim not found in id_token")
	ErrNameClaimNotFound      = errors.New("name claim not found in id_token")
	ErrUnsupportedEmailDomain = errors.New("경희대학교 계정(@khu.ac.kr)으로만 로그인할 수 있습니다")
	ErrAccessTokenNotFound    = errors.New("access token not found")
	ErrInvalidAccessToken     = errors.New("invalid access token")
	ErrRefreshTokenNotFound   = errors.New("refresh token not found")
	ErrInvalidRefreshToken    = errors.New("invalid refresh token")
	ErrInvalidTokenType       = errors.New("invalid token type")
	ErrUserNotFound           = errors.New("user not found")
	ErrUserLookupFailed       = errors.New("failed to verify user")
	ErrInvalidOAuthState      = errors.New("invalid oauth state")
)

// userRepository는 유저 데이터 접근 인터페이스입니다.
// 구현체는 repository.go의 Repository입니다.
type userRepository interface {
	UpsertUser(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
	SetRefreshJTI(ctx context.Context, email string, jti *string) error
	RotateRefreshJTI(ctx context.Context, email string, oldJTI, newJTI string) (bool, error)
	ClearRefreshJTIIfMatches(ctx context.Context, email, expectedJTI string) (bool, error)
}

// Service는 인증 비즈니스 로직을 담당합니다.
type Service struct {
	repo         userRepository
	OauthConfig  *oauth2.Config
	TokenService *TokenService // JWT 발급을 위해 주입받음
}

// NewService는 새로운 서비스를 생성합니다.
func NewService(repo userRepository, config *oauth2.Config, svc *TokenService) *Service {
	return &Service{
		repo:         repo,
		OauthConfig:  config,
		TokenService: svc,
	}
}
