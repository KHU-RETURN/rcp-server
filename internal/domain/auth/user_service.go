package auth

import (
	"context"
	"fmt"
)

func (s *Service) GetUserByAccessToken(ctx context.Context, token string) (*User, error) {
	claims, err := s.TokenService.ValidateToken(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAccessToken, err)
	}
	if claims.Type != tokenTypeAccess {
		return nil, ErrInvalidTokenType
	}

	user, err := s.repo.FindByEmail(ctx, claims.Email)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserLookupFailed, err)
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	return user, nil
}

// RefreshAccessToken은 유효한 refresh token으로 새로운 access+refresh 토큰을 발급합니다(회전).
// 제시된 refresh token의 jti가 서버에 저장된 활성 jti와 일치할 때만 성공합니다.
// 회전 후 서버에 저장된 jti는 새 토큰의 jti로 교체되어 이전 refresh token은 재사용 불가가 됩니다.
func (s *Service) RefreshAccessToken(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.TokenService.ValidateToken(refreshToken)
	if err != nil {
		return TokenPair{}, fmt.Errorf("%w: %v", ErrInvalidRefreshToken, err)
	}
	if claims.Type != tokenTypeRefresh {
		return TokenPair{}, ErrInvalidTokenType
	}
	if claims.ID == "" {
		return TokenPair{}, ErrInvalidRefreshToken
	}

	user, err := s.repo.FindByEmail(ctx, claims.Email)
	if err != nil {
		return TokenPair{}, fmt.Errorf("%w: %v", ErrUserLookupFailed, err)
	}
	if user == nil {
		return TokenPair{}, ErrUserNotFound
	}
	if user.CurrentRefreshJTI == nil || *user.CurrentRefreshJTI != claims.ID {
		return TokenPair{}, ErrInvalidRefreshToken
	}

	tokens, err := s.TokenService.GenerateAuthTokens(user.Email)
	if err != nil {
		return TokenPair{}, fmt.Errorf("failed to generate new tokens: %w", err)
	}

	// compare-and-set으로 회전. 동시에 같은 refresh token으로 들어온 다른 요청이
	// 먼저 회전시켰다면 rotated=false → 이 요청은 stale이므로 거절.
	rotated, err := s.repo.RotateRefreshJTI(ctx, user.Email, claims.ID, tokens.RefreshJTI)
	if err != nil {
		return TokenPair{}, fmt.Errorf("%w: %v", ErrUserLookupFailed, err)
	}
	if !rotated {
		return TokenPair{}, ErrInvalidRefreshToken
	}

	return tokens, nil
}

// Logout은 refresh token이 유효하면 해당 user의 서버측 jti를 비웁니다.
// 토큰이 유효하지 않거나 없어도 에러를 반환하지 않습니다(graceful logout).
// 반환되는 bool은 서버측 무효화가 실제로 일어났는지를 알려줍니다.
func (s *Service) Logout(ctx context.Context, refreshToken string) (bool, error) {
	if refreshToken == "" {
		return false, nil
	}
	claims, err := s.TokenService.ValidateToken(refreshToken)
	if err != nil || claims.Type != tokenTypeRefresh || claims.Email == "" {
		return false, nil
	}
	if err := s.repo.SetRefreshJTI(ctx, claims.Email, nil); err != nil {
		return false, fmt.Errorf("%w: %v", ErrUserLookupFailed, err)
	}
	return true, nil
}
