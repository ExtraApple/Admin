package jwtadapter

import (
	"context"
	"fmt"
	"time"

	"admin/internal/identity/application"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	Secret                  string
	AccessExpireMins        int
	RefreshExpireMins       int
	LegacyAccessExpireMins  int
	LegacyRefreshExpireMins int
}

type Service struct{ config Config }

func NewService(config Config) *Service { return &Service{config: config} }

type claims struct {
	UserID       uint                     `json:"user_id"`
	TokenVersion int                      `json:"token_version"`
	TokenPurpose application.TokenPurpose `json:"token_type,omitempty"`
	Roles        []string                 `json:"roles"`
	Permissions  []string                 `json:"permissions"`
	jwt.RegisteredClaims
}

func (service *Service) Issue(_ context.Context, issue application.TokenIssue) (application.TokenPair, error) {
	now := time.Now()
	access, err := service.sign(claims{
		UserID: issue.UserID, TokenVersion: issue.TokenVersion, TokenPurpose: application.TokenPurposeAccess,
		Roles: append([]string(nil), issue.Roles...), Permissions: append([]string(nil), issue.Permissions...),
		RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(service.config.AccessExpireMins) * time.Minute))},
	})
	if err != nil {
		return application.TokenPair{}, fmt.Errorf("generate access token failed: %w", err)
	}
	refresh, err := service.sign(claims{
		UserID: issue.UserID, TokenVersion: issue.TokenVersion, TokenPurpose: application.TokenPurposeRefresh,
		Roles: append([]string(nil), issue.Roles...), Permissions: append([]string(nil), issue.Permissions...),
		RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(service.config.RefreshExpireMins) * time.Minute))},
	})
	if err != nil {
		return application.TokenPair{}, fmt.Errorf("generate refresh token failed: %w", err)
	}
	return application.TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (service *Service) Parse(_ context.Context, tokenString string) (application.TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(service.config.Secret), nil
	})
	if err != nil {
		return application.TokenClaims{}, fmt.Errorf("parse token failed: %w", err)
	}
	parsed, ok := token.Claims.(*claims)
	if !ok || !token.Valid {
		return application.TokenClaims{}, fmt.Errorf("invalid token")
	}
	purpose := parsed.TokenPurpose
	if purpose == "" {
		purpose = service.legacyPurpose(parsed)
	}
	return application.TokenClaims{UserID: parsed.UserID, TokenVersion: parsed.TokenVersion, Purpose: purpose}, nil
}

func (service *Service) sign(claims claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(service.config.Secret))
}

func (service *Service) legacyPurpose(parsed *claims) application.TokenPurpose {
	if parsed.IssuedAt == nil || parsed.ExpiresAt == nil || service.config.LegacyAccessExpireMins <= 0 || service.config.LegacyRefreshExpireMins <= service.config.LegacyAccessExpireMins {
		return ""
	}
	lifetime := parsed.ExpiresAt.Time.Sub(parsed.IssuedAt.Time)
	switch lifetime {
	case time.Duration(service.config.LegacyAccessExpireMins) * time.Minute:
		return application.TokenPurposeAccess
	case time.Duration(service.config.LegacyRefreshExpireMins) * time.Minute:
		return application.TokenPurposeRefresh
	default:
		return ""
	}
}

var _ application.TokenService = (*Service)(nil)
