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
	if claims.Type != "access" {
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
