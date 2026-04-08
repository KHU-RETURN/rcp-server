package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

type User struct {
	ID    uuid.UUID
	Email string
	Name  string
	// AccessToken, RefreshToken, Expiry는 로그인 응답 시 transient하게 사용됩니다.
	// DB에는 Session 엔티티로 분리 저장됩니다.
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

type Session struct {
	ID              uuid.UUID
	AccessToken     string
	RefreshToken    string
	Expiry          time.Time
	Provider        string
	ProviderToken   string
	ProviderRefresh *string
	ProviderExpiry  *time.Time
	CreatedAt       time.Time
}

// MyClaims는 JWT 페이로드에 담길 우리 서비스 전용 정보입니다.
type MyClaims struct {
	Email string `json:"email"`
	Type  string `json:"type"` // "access" 또는 "refresh"
	jwt.RegisteredClaims
}
