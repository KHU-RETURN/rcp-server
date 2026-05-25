package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	accessTokenTTL  = 1 * time.Hour
	refreshTokenTTL = 14 * 24 * time.Hour

	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
	jwtIssuer        = "rcp-auth-service"
)

// TokenService는 JWT 토큰의 생성 및 검증을 담당합니다.
type TokenService struct {
	SecretKey []byte
}

// TokenPair는 새로 발급된 access/refresh 토큰과 메타데이터를 묶어 반환합니다.
type TokenPair struct {
	AccessToken  string
	AccessExpiry time.Time
	RefreshToken string
	RefreshJTI   string
}

// NewTokenService는 새로운 토큰 서비스를 생성합니다.
func NewTokenService(secret string) *TokenService {
	return &TokenService{
		SecretKey: []byte(secret),
	}
}

// GenerateAuthTokens는 유저의 이메일을 기반으로 Access와 Refresh 토큰 쌍을 생성합니다.
// 반환된 RefreshJTI는 서버측 회전/무효화 추적용으로 저장해야 합니다.
func (s *TokenService) GenerateAuthTokens(email string) (TokenPair, error) {
	accessToken, accessExpiry, err := s.GenerateAccessToken(email)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, refreshJTI, err := s.GenerateRefreshToken(email)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessToken,
		AccessExpiry: accessExpiry,
		RefreshToken: refreshToken,
		RefreshJTI:   refreshJTI,
	}, nil
}

// GenerateAccessToken은 Access Token만 새로 발급합니다. Refresh 갱신 흐름에서 사용합니다.
func (s *TokenService) GenerateAccessToken(email string) (string, time.Time, error) {
	expiry := time.Now().Add(accessTokenTTL)
	token, err := s.createToken(email, tokenTypeAccess, expiry, "")
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiry, nil
}

// GenerateRefreshToken은 새 jti를 부여한 refresh token을 발급합니다.
func (s *TokenService) GenerateRefreshToken(email string) (string, string, error) {
	jti := uuid.NewString()
	expiry := time.Now().Add(refreshTokenTTL)
	token, err := s.createToken(email, tokenTypeRefresh, expiry, jti)
	if err != nil {
		return "", "", err
	}
	return token, jti, nil
}

// createToken은 공통적인 JWT 생성 로직을 처리합니다. jti가 비어있으면 ID 클레임을 생략합니다.
func (s *TokenService) createToken(email, tokenType string, expiry time.Time, jti string) (string, error) {
	claims := MyClaims{
		Email: email,
		Type:  tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    jwtIssuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.SecretKey)
}

// ValidateToken은 전달받은 토큰의 유효성을 검사하고 클레임을 반환합니다.
func (s *TokenService) ValidateToken(tokenString string) (*MyClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MyClaims{}, func(t *jwt.Token) (any, error) {
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
