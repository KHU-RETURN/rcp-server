package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	GoogleID     string    `json:"-"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	// 구글 전용 정보 (서버 내부에서만 사용, 클라이언트에 노출 안 함)
	GoogleAuth *GoogleInfo `json:"-"`
}

type MeResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	AccessToken string    `json:"access_token"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type GoogleInfo struct {
	AccessToken  string    `json:"google_access_token"`
	RefreshToken string    `json:"google_refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// MyClaims는 JWT 페이로드에 담길 우리 서비스 전용 정보입니다.
type MyClaims struct {
	Email string `json:"email"`
	Type  string `json:"type"` // "access" 또는 "refresh"
	jwt.RegisteredClaims
}
