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
	Role         string    `json:"role"`
	GoogleID     string    `json:"-"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	// CurrentRefreshJTI는 서버측에서 추적하는 활성 refresh token 식별자입니다.
	// nil이면 활성 세션 없음(logout 상태). 회전 시 매번 교체됩니다.
	CurrentRefreshJTI *string `json:"-"`
	// 구글 전용 정보 (서버 내부에서만 사용, 클라이언트에 노출 안 함)
	GoogleAuth *GoogleInfo `json:"-"`
}

type MeResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	Role        string    `json:"role"`
	AccessToken string    `json:"access_token"`
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
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
