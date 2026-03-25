package auth

import (
	"context"
	"testing"
	"golang.org/x/oauth2"
)

// 1. Mock 객체 정의: UserRepository 인터페이스의 모든 메서드를 구현해야 합니다.
type MockUserRepository struct {
	Users map[string]*User
}

// UpsertUser 구현
func (m *MockUserRepository) UpsertUser(ctx context.Context, user *User) error {
	m.Users[user.Email] = user
	return nil
}

// ⚠️ 추가된 부분: FindByEmail 메서드 구현 (인터페이스 요구사항 충족)
func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	user, ok := m.Users[email]
	if !ok {
		return nil, nil // 혹은 errors.New("not found")
	}
	return user, nil
}

func TestService_Initialization(t *testing.T) {
	// 2. 준비
	mockRepo := &MockUserRepository{Users: make(map[string]*User)}
	tokenSvc := NewTokenService("test-secret-key")
	conf := &oauth2.Config{}

	// 3. 실행 (컴파일 에러 해결 지점)
	authSvc := NewService(mockRepo, conf, tokenSvc)

	// 4. 검증 (변수 미사용 에러 해결: authSvc를 실제로 테스트에 사용)
	if authSvc == nil {
		t.Fatal("authSvc should not be nil")
	}
	
	if authSvc.TokenService == nil {
		t.Error("TokenService was not properly injected")
	}
}

func TestTokenService_Logic(t *testing.T) {
	svc := NewTokenService("secret")
	t.Run("Generate Tokens", func(t *testing.T) {
		_, _, _, err := svc.GenerateAuthTokens("test@khu.ac.kr")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}