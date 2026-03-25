package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)


// TokenService는 JWT 토큰의 생성 및 검증을 담당합니다.
type TokenService struct {
	SecretKey []byte
}

// NewTokenService는 새로운 토큰 서비스를 생성합니다.
func NewTokenService(secret string) *TokenService {
	return &TokenService{
		SecretKey: []byte(secret),
	}
}

// GenerateAuthTokens는 유저의 이메일을 기반으로 Access와 Refresh 토큰 쌍을 생성합니다.
func (s *TokenService) GenerateAuthTokens(email string) (string, string, time.Time, error) {
	// 1. Access Token 생성 (1시간 유효)
	accessTokenExpiry := time.Now().Add(1 * time.Hour)
	accessToken, err := s.createToken(email, "access", accessTokenExpiry)
	if err != nil {
		return "", "", time.Time{}, err
	}

	// 2. Refresh Token 생성 (14일 유효)
	refreshTokenExpiry := time.Now().Add(14 * 24 * time.Hour)
	refreshToken, err := s.createToken(email, "refresh", refreshTokenExpiry)
	if err != nil {
		return "", "", time.Time{}, err
	}

	return accessToken, refreshToken, accessTokenExpiry, nil
}

// createToken은 공통적인 JWT 생성 로직을 처리합니다.
func (s *TokenService) createToken(email, tokenType string, expiry time.Time) (string, error) {
	claims := MyClaims{
		Email: email,
		Type:  tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "rcp-auth-service",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.SecretKey)
}

// ValidateToken은 전달받은 토큰의 유효성을 검사하고 클레임을 반환합니다.
func (s *TokenService) ValidateToken(tokenString string) (*MyClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MyClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.SecretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*MyClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}