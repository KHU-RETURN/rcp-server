package auth

import (
	"context"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type fakeRepo struct {
	users map[string]*User
}

func (f *fakeRepo) UpsertUser(ctx context.Context, user *User) error {
	f.users[user.Email] = user
	return nil
}

func (f *fakeRepo) FindByEmail(ctx context.Context, email string) (*User, error) {
	user, ok := f.users[email]
	if !ok {
		return nil, nil
	}
	return user, nil
}

// MockUserRepository는 이전 테스트 코드와의 호환을 위해 유지합니다.
type MockUserRepository = fakeRepo

func TestService_Initialization(t *testing.T) {
	mockRepo := &fakeRepo{users: make(map[string]*User)}
	tokenSvc := NewTokenService("test-secret-key")
	conf := &oauth2.Config{}

	authSvc := NewService(mockRepo, conf, tokenSvc)

	if authSvc == nil {
		t.Fatal("authSvc should not be nil")
	}
	if authSvc.TokenService == nil {
		t.Error("TokenService was not properly injected")
	}
}

func TestTokenService(t *testing.T) {
	svc := NewTokenService("test-secret")

	t.Run("generates access and refresh tokens without error", func(t *testing.T) {
		access, refresh, expiry, err := svc.GenerateAuthTokens("test@khu.ac.kr")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if access == "" {
			t.Fatal("expected non-empty access token")
		}
		if refresh == "" {
			t.Fatal("expected non-empty refresh token")
		}
		if expiry.IsZero() {
			t.Fatal("expected non-zero expiry")
		}
	})

	t.Run("access token expiry is approximately 1 hour from now", func(t *testing.T) {
		_, _, expiry, err := svc.GenerateAuthTokens("test@khu.ac.kr")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		remaining := time.Until(expiry)
		if remaining < 59*time.Minute || remaining > 61*time.Minute {
			t.Fatalf("expected expiry ~1 hour from now, got %v remaining", remaining)
		}
	})

	t.Run("access token validates with correct claims", func(t *testing.T) {
		email := "user@khu.ac.kr"
		access, _, _, err := svc.GenerateAuthTokens(email)
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		claims, err := svc.ValidateToken(access)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if claims.Email != email {
			t.Fatalf("expected email %q, got %q", email, claims.Email)
		}
		if claims.Type != "access" {
			t.Fatalf("expected type 'access', got %q", claims.Type)
		}
	})

	t.Run("refresh token validates with correct claims", func(t *testing.T) {
		_, refresh, _, err := svc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		claims, err := svc.ValidateToken(refresh)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if claims.Type != "refresh" {
			t.Fatalf("expected type 'refresh', got %q", claims.Type)
		}
	})

	t.Run("rejects token signed with different secret", func(t *testing.T) {
		otherSvc := NewTokenService("different-secret")
		token, _, _, err := otherSvc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		_, err = svc.ValidateToken(token)
		if err == nil {
			t.Fatal("expected error for token with wrong secret, got nil")
		}
	})

	t.Run("rejects tampered token", func(t *testing.T) {
		_, err := svc.ValidateToken("not.a.valid.jwt")
		if err == nil {
			t.Fatal("expected error for malformed token, got nil")
		}
	})
}
