package utils

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenPurpose string

const (
	TokenPurposeAccess  TokenPurpose = "access"
	TokenPurposeRefresh TokenPurpose = "refresh"
)

type LegacyTokenPurposeConfig struct {
	AccessExpireMins  int
	RefreshExpireMins int
}

type Claims struct {
	UserID       uint         `json:"user_id"`
	TokenVersion int          `json:"token_version"`
	TokenPurpose TokenPurpose `json:"token_type,omitempty"`
	Roles        []string     `json:"roles"`
	Permissions  []string     `json:"permissions"`
	jwt.RegisteredClaims
}

func (c *Claims) HasPurpose(
	expected TokenPurpose,
	legacy LegacyTokenPurposeConfig,
) bool {
	if c.TokenPurpose != "" {
		return c.TokenPurpose == expected
	}
	if c.IssuedAt == nil || c.ExpiresAt == nil ||
		legacy.AccessExpireMins <= 0 ||
		legacy.RefreshExpireMins <= legacy.AccessExpireMins {
		return false
	}

	lifetime := c.ExpiresAt.Time.Sub(c.IssuedAt.Time)
	switch expected {
	case TokenPurposeAccess:
		return lifetime == time.Duration(legacy.AccessExpireMins)*time.Minute
	case TokenPurposeRefresh:
		return lifetime == time.Duration(legacy.RefreshExpireMins)*time.Minute
	default:
		return false
	}
}

// GenerateToken 生成 access token（短期）和 refresh token（长期）
func GenerateToken(userID uint, tokenVersion int, roles, permissions []string, secret string, expireMins int, refreshExpireMins int) (string, string, error) {
	now := time.Now()

	accessClaims := Claims{
		UserID:       userID,
		TokenVersion: tokenVersion,
		TokenPurpose: TokenPurposeAccess,
		Roles:        roles,
		Permissions:  permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireMins) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(secret))
	if err != nil {
		return "", "", fmt.Errorf("generate access token failed: %w", err)
	}

	refreshClaims := Claims{
		UserID:       userID,
		TokenVersion: tokenVersion,
		TokenPurpose: TokenPurposeRefresh,
		Roles:        roles,
		Permissions:  permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(refreshExpireMins) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(secret))
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token failed: %w", err)
	}

	return accessToken, refreshToken, nil
}

// ParseToken 解析并验证 token，返回 claims
func ParseToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token failed: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
