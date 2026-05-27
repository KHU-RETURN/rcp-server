package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type fakeRepo struct {
	users              map[string]*User
	findByEmailErr     error
	clearRefreshJTIErr error
}

func (f *fakeRepo) UpsertUser(ctx context.Context, user *User) error {
	f.users[user.Email] = user
	return nil
}

func (f *fakeRepo) FindByEmail(ctx context.Context, email string) (*User, error) {
	if f.findByEmailErr != nil {
		return nil, f.findByEmailErr
	}
	user, ok := f.users[email]
	if !ok {
		return nil, nil
	}
	return user, nil
}

func (f *fakeRepo) SetRefreshJTI(ctx context.Context, email string, jti *string) error {
	user, ok := f.users[email]
	if !ok {
		return nil
	}
	user.CurrentRefreshJTI = jti
	return nil
}

func (f *fakeRepo) RotateRefreshJTI(ctx context.Context, email string, oldJTI, newJTI string) (bool, error) {
	user, ok := f.users[email]
	if !ok {
		return false, nil
	}
	if user.CurrentRefreshJTI == nil || *user.CurrentRefreshJTI != oldJTI {
		return false, nil
	}
	user.CurrentRefreshJTI = &newJTI
	return true, nil
}

func (f *fakeRepo) ClearRefreshJTIIfMatches(ctx context.Context, email, expectedJTI string) (bool, error) {
	if f.clearRefreshJTIErr != nil {
		return false, f.clearRefreshJTIErr
	}
	user, ok := f.users[email]
	if !ok {
		return false, nil
	}
	if user.CurrentRefreshJTI == nil || *user.CurrentRefreshJTI != expectedJTI {
		return false, nil
	}
	user.CurrentRefreshJTI = nil
	return true, nil
}

// MockUserRepository는 이전 테스트 코드와의 호환을 위해 유지합니다.
type MockUserRepository = fakeRepo

func issueAndStore(t *testing.T, repo *fakeRepo, tokenSvc *TokenService, email string) TokenPair {
	t.Helper()
	pair, err := tokenSvc.GenerateAuthTokens(email)
	if err != nil {
		t.Fatalf("GenerateAuthTokens: %v", err)
	}
	if user, ok := repo.users[email]; ok {
		jti := pair.RefreshJTI
		user.CurrentRefreshJTI = &jti
	}
	return pair
}

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
		pair, err := svc.GenerateAuthTokens("test@khu.ac.kr")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if pair.AccessToken == "" {
			t.Fatal("expected non-empty access token")
		}
		if pair.RefreshToken == "" {
			t.Fatal("expected non-empty refresh token")
		}
		if pair.RefreshJTI == "" {
			t.Fatal("expected non-empty refresh jti")
		}
		if pair.AccessExpiry.IsZero() {
			t.Fatal("expected non-zero access expiry")
		}
	})

	t.Run("access token expiry is approximately 1 hour from now", func(t *testing.T) {
		pair, err := svc.GenerateAuthTokens("test@khu.ac.kr")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		remaining := time.Until(pair.AccessExpiry)
		if remaining < 59*time.Minute || remaining > 61*time.Minute {
			t.Fatalf("expected expiry ~1 hour from now, got %v remaining", remaining)
		}
	})

	t.Run("access token validates with correct claims", func(t *testing.T) {
		email := "user@khu.ac.kr"
		pair, err := svc.GenerateAuthTokens(email)
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		claims, err := svc.ValidateToken(pair.AccessToken)
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

	t.Run("refresh token validates with correct claims and jti", func(t *testing.T) {
		pair, err := svc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		claims, err := svc.ValidateToken(pair.RefreshToken)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if claims.Type != "refresh" {
			t.Fatalf("expected type 'refresh', got %q", claims.Type)
		}
		if claims.ID == "" {
			t.Fatal("expected refresh token to carry jti in ID claim")
		}
		if claims.ID != pair.RefreshJTI {
			t.Fatalf("expected jti %q in token, got %q", pair.RefreshJTI, claims.ID)
		}
	})

	t.Run("every refresh issuance produces a unique jti", func(t *testing.T) {
		first, err := svc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}
		second, err := svc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}
		if first.RefreshJTI == second.RefreshJTI {
			t.Fatal("expected distinct jti per issuance")
		}
	})

	t.Run("rejects token signed with different secret", func(t *testing.T) {
		otherSvc := NewTokenService("different-secret")
		pair, err := otherSvc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		if _, err := svc.ValidateToken(pair.AccessToken); err == nil {
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

func TestServiceGetUserByAccessToken(t *testing.T) {
	ctx := context.Background()
	tokenSvc := NewTokenService("test-secret")
	user := &User{Email: "user@khu.ac.kr", Name: "User"}
	repo := &fakeRepo{users: map[string]*User{user.Email: user}}
	svc := NewService(repo, &oauth2.Config{}, tokenSvc)

	t.Run("returns user for valid access token", func(t *testing.T) {
		pair := issueAndStore(t, repo, tokenSvc, user.Email)

		got, err := svc.GetUserByAccessToken(ctx, pair.AccessToken)
		if err != nil {
			t.Fatalf("GetUserByAccessToken: %v", err)
		}
		if got != user {
			t.Fatalf("expected user %+v, got %+v", user, got)
		}
	})

	t.Run("rejects refresh token", func(t *testing.T) {
		pair := issueAndStore(t, repo, tokenSvc, user.Email)

		if _, err := svc.GetUserByAccessToken(ctx, pair.RefreshToken); !errors.Is(err, ErrInvalidTokenType) {
			t.Fatalf("expected ErrInvalidTokenType, got %v", err)
		}
	})

	t.Run("rejects missing user", func(t *testing.T) {
		emptyRepo := &fakeRepo{users: map[string]*User{}}
		emptySvc := NewService(emptyRepo, &oauth2.Config{}, tokenSvc)
		pair, err := tokenSvc.GenerateAuthTokens("missing@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		if _, err := emptySvc.GetUserByAccessToken(ctx, pair.AccessToken); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("returns repository error", func(t *testing.T) {
		repoErr := errors.New("repository failed")
		errRepo := &fakeRepo{users: map[string]*User{}, findByEmailErr: repoErr}
		errSvc := NewService(errRepo, &oauth2.Config{}, tokenSvc)
		pair, err := tokenSvc.GenerateAuthTokens(user.Email)
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		if _, err := errSvc.GetUserByAccessToken(ctx, pair.AccessToken); !errors.Is(err, ErrUserLookupFailed) {
			t.Fatalf("expected ErrUserLookupFailed, got %v", err)
		}
	})
}

func TestServiceRefreshAccessToken(t *testing.T) {
	ctx := context.Background()
	tokenSvc := NewTokenService("test-secret")

	newSvc := func(email string) (*Service, *fakeRepo) {
		repo := &fakeRepo{users: map[string]*User{
			email: {Email: email, Name: "User"},
		}}
		return NewService(repo, &oauth2.Config{}, tokenSvc), repo
	}

	t.Run("rotates jti and issues new token pair", func(t *testing.T) {
		svc, repo := newSvc("user@khu.ac.kr")
		original := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")

		newPair, err := svc.RefreshAccessToken(ctx, original.RefreshToken)
		if err != nil {
			t.Fatalf("RefreshAccessToken: %v", err)
		}
		if newPair.AccessToken == "" || newPair.RefreshToken == "" {
			t.Fatal("expected non-empty new tokens")
		}
		if newPair.RefreshJTI == original.RefreshJTI {
			t.Fatal("expected rotated refresh jti, got same as original")
		}
		stored := repo.users["user@khu.ac.kr"].CurrentRefreshJTI
		if stored == nil || *stored != newPair.RefreshJTI {
			t.Fatalf("expected stored jti to be rotated to %q, got %v", newPair.RefreshJTI, stored)
		}
		if time.Until(newPair.AccessExpiry) < 59*time.Minute {
			t.Fatalf("expected expiry ~1 hour from now, got %v", newPair.AccessExpiry)
		}
	})

	t.Run("rejects old refresh token after rotation (replay)", func(t *testing.T) {
		svc, repo := newSvc("user@khu.ac.kr")
		original := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")

		if _, err := svc.RefreshAccessToken(ctx, original.RefreshToken); err != nil {
			t.Fatalf("first refresh: %v", err)
		}
		if _, err := svc.RefreshAccessToken(ctx, original.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("expected ErrInvalidRefreshToken on replay, got %v", err)
		}
	})

	t.Run("rejects refresh when server-side jti is cleared (post-logout)", func(t *testing.T) {
		svc, repo := newSvc("user@khu.ac.kr")
		original := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")
		repo.users["user@khu.ac.kr"].CurrentRefreshJTI = nil

		if _, err := svc.RefreshAccessToken(ctx, original.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
		}
	})

	t.Run("rejects access token used as refresh", func(t *testing.T) {
		svc, repo := newSvc("user@khu.ac.kr")
		pair := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")

		if _, err := svc.RefreshAccessToken(ctx, pair.AccessToken); !errors.Is(err, ErrInvalidTokenType) {
			t.Fatalf("expected ErrInvalidTokenType, got %v", err)
		}
	})

	t.Run("rejects malformed token", func(t *testing.T) {
		svc, _ := newSvc("user@khu.ac.kr")
		if _, err := svc.RefreshAccessToken(ctx, "not.a.jwt"); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("expected ErrInvalidRefreshToken, got %v", err)
		}
	})

	t.Run("rejects when user no longer exists", func(t *testing.T) {
		svc, _ := newSvc("user@khu.ac.kr")
		pair, err := tokenSvc.GenerateAuthTokens("ghost@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		if _, err := svc.RefreshAccessToken(ctx, pair.RefreshToken); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})

	// 같은 refresh token으로 동시에 두 요청이 들어왔을 때, 한쪽이 먼저 회전을 끝내면
	// 다른 쪽은 stale로 거절돼야 한다. read-modify-write에서 발생할 수 있는 양쪽 모두 성공 케이스 방지.
	t.Run("rejects refresh when jti was rotated by concurrent request", func(t *testing.T) {
		svc, repo := newSvc("user@khu.ac.kr")
		original := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")

		// 다른 요청이 먼저 회전을 끝낸 상황을 시뮬레이션: 저장된 jti만 바뀐 상태.
		otherJTI := "concurrent-winner-jti"
		repo.users["user@khu.ac.kr"].CurrentRefreshJTI = &otherJTI

		// 이 시점에서 원래 refresh token은 JWT-valid + 서명 OK이지만 jti는 stale.
		if _, err := svc.RefreshAccessToken(ctx, original.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("expected ErrInvalidRefreshToken on concurrent rotation, got %v", err)
		}
		// 다른 요청의 jti가 덮어쓰여서는 안 됨.
		stored := repo.users["user@khu.ac.kr"].CurrentRefreshJTI
		if stored == nil || *stored != otherJTI {
			t.Fatalf("concurrent winner's jti was overwritten: got %v, want %q", stored, otherJTI)
		}
	})
}

func TestServiceLogout(t *testing.T) {
	ctx := context.Background()
	tokenSvc := NewTokenService("test-secret")

	t.Run("clears stored jti for valid refresh", func(t *testing.T) {
		email := "user@khu.ac.kr"
		repo := &fakeRepo{users: map[string]*User{email: {Email: email}}}
		svc := NewService(repo, &oauth2.Config{}, tokenSvc)
		pair := issueAndStore(t, repo, tokenSvc, email)

		invalidated, err := svc.Logout(ctx, pair.RefreshToken)
		if err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if !invalidated {
			t.Fatal("expected invalidated=true for valid refresh")
		}
		if repo.users[email].CurrentRefreshJTI != nil {
			t.Fatal("expected stored jti to be cleared")
		}
	})

	t.Run("succeeds without invalidation when refresh is missing", func(t *testing.T) {
		repo := &fakeRepo{users: map[string]*User{}}
		svc := NewService(repo, &oauth2.Config{}, tokenSvc)

		invalidated, err := svc.Logout(ctx, "")
		if err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if invalidated {
			t.Fatal("expected invalidated=false for empty token")
		}
	})

	t.Run("succeeds without invalidation when token is malformed", func(t *testing.T) {
		repo := &fakeRepo{users: map[string]*User{}}
		svc := NewService(repo, &oauth2.Config{}, tokenSvc)

		invalidated, err := svc.Logout(ctx, "not.a.jwt")
		if err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if invalidated {
			t.Fatal("expected invalidated=false for malformed token")
		}
	})

	t.Run("succeeds without invalidation when access token presented", func(t *testing.T) {
		email := "user@khu.ac.kr"
		repo := &fakeRepo{users: map[string]*User{email: {Email: email}}}
		svc := NewService(repo, &oauth2.Config{}, tokenSvc)
		pair := issueAndStore(t, repo, tokenSvc, email)

		invalidated, err := svc.Logout(ctx, pair.AccessToken)
		if err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if invalidated {
			t.Fatal("expected invalidated=false when access token is used")
		}
		if repo.users[email].CurrentRefreshJTI == nil {
			t.Fatal("expected stored jti to remain intact")
		}
	})

	// stale(이미 회전된) refresh token으로는 현재 활성 세션을 종료시킬 수 없어야 한다.
	// 옛 토큰이 잠깐 누출돼도 공격자가 user의 활성 세션을 DoS하는 걸 막는다.
	t.Run("does not invalidate active session when stale refresh presented", func(t *testing.T) {
		email := "user@khu.ac.kr"
		repo := &fakeRepo{users: map[string]*User{email: {Email: email}}}
		svc := NewService(repo, &oauth2.Config{}, tokenSvc)
		stale := issueAndStore(t, repo, tokenSvc, email)

		// 다른 디바이스에서 정상 refresh로 jti가 회전된 상황
		activeJTI := "new-active-jti"
		repo.users[email].CurrentRefreshJTI = &activeJTI

		invalidated, err := svc.Logout(ctx, stale.RefreshToken)
		if err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if invalidated {
			t.Fatal("expected invalidated=false for stale refresh token")
		}
		stored := repo.users[email].CurrentRefreshJTI
		if stored == nil || *stored != activeJTI {
			t.Fatalf("active session jti was wiped by stale token: got %v, want %q", stored, activeJTI)
		}
	})
}
