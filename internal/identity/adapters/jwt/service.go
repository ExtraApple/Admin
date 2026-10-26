package jwtadapter

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"admin/internal/identity/application"
	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	Secret            string
	AccessExpireMins  int
	RefreshExpireMins int
}

type Service struct{ config Config }

func NewService(config Config) *Service { return &Service{config: config} }

type sessionClaims struct {
	UserID       uint                     `json:"user_id"`
	TokenVersion int                      `json:"token_version"`
	TokenPurpose application.TokenPurpose `json:"token_type"`
	SessionID    string                   `json:"sid"`
	Generation   int64                    `json:"refresh_generation,omitempty"`
	jwt.RegisteredClaims
}

type claims struct {
	sessionClaims
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

func randomIdentifier() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func (service *Service) Prepare(_ context.Context, issue application.TokenIssue) (application.TokenIssue, error) {
	if service.config.Secret == "" || service.config.AccessExpireMins <= 0 || service.config.RefreshExpireMins <= 0 || issue.UserID == 0 || issue.TokenVersion <= 0 {
		return application.TokenIssue{}, fmt.Errorf("invalid token signing configuration or binding")
	}
	var err error
	if issue.SessionID == "" {
		issue.SessionID, err = randomIdentifier()
		if err != nil {
			return application.TokenIssue{}, err
		}
	}
	if issue.RefreshID == "" {
		issue.RefreshID, err = randomIdentifier()
		if err != nil {
			return application.TokenIssue{}, err
		}
	}
	if issue.Generation == 0 {
		issue.Generation = 1
	}
	if issue.IssuedAt.IsZero() {
		issue.IssuedAt = time.Now().UTC().Truncate(time.Second)
		issue.AccessExpiresAt = issue.IssuedAt.Add(time.Duration(service.config.AccessExpireMins) * time.Minute)
		issue.RefreshExpiresAt = issue.IssuedAt.Add(time.Duration(service.config.RefreshExpireMins) * time.Minute)
	}
	if !application.ValidSessionIdentifier(issue.SessionID) || !application.ValidSessionIdentifier(issue.RefreshID) || issue.Generation < 1 || !issue.AccessExpiresAt.After(issue.IssuedAt) || !issue.RefreshExpiresAt.After(issue.IssuedAt) {
		return application.TokenIssue{}, fmt.Errorf("invalid fixed signing material")
	}
	return issue, nil
}

func (service *Service) Issue(ctx context.Context, issue application.TokenIssue) (application.TokenPair, error) {
	issue, err := service.Prepare(ctx, issue)
	if err != nil {
		return application.TokenPair{}, err
	}
	base := sessionClaims{UserID: issue.UserID, TokenVersion: issue.TokenVersion, SessionID: issue.SessionID,
		RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(issue.IssuedAt), ExpiresAt: jwt.NewNumericDate(issue.AccessExpiresAt)}}
	base.TokenPurpose = application.TokenPurposeAccess
	access, err := service.sign(claims{sessionClaims: base, Roles: issue.Roles, Permissions: issue.Permissions})
	if err != nil {
		return application.TokenPair{}, err
	}
	base.TokenPurpose = application.TokenPurposeRefresh
	base.Generation = issue.Generation
	base.ID = issue.RefreshID
	base.ExpiresAt = jwt.NewNumericDate(issue.RefreshExpiresAt)
	refresh, err := service.sign(base)
	if err != nil {
		return application.TokenPair{}, err
	}
	return application.TokenPair{AccessToken: access, RefreshToken: refresh, SessionID: issue.SessionID, Material: issue}, nil
}

func (service *Service) Parse(ctx context.Context, token string) (application.TokenClaims, error) {
	return service.parse(token, false)
}

func (service *Service) ParseRefresh(ctx context.Context, token string) (application.TokenClaims, error) {
	return service.parse(token, true)
}

func (service *Service) parse(tokenString string, recoverExpired bool) (application.TokenClaims, error) {
	options := []jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt()}
	if recoverExpired {
		options = append(options, jwt.WithoutClaimsValidation())
	}
	token, err := jwt.ParseWithClaims(tokenString, &claims{}, func(token *jwt.Token) (any, error) { return []byte(service.config.Secret), nil }, options...)
	if err != nil {
		return application.TokenClaims{}, fmt.Errorf("parse token failed: %w", err)
	}
	parsed, ok := token.Claims.(*claims)
	if !ok || !token.Valid || parsed.UserID == 0 || parsed.TokenVersion <= 0 || !application.ValidSessionIdentifier(parsed.SessionID) || parsed.IssuedAt == nil || parsed.ExpiresAt == nil || !parsed.ExpiresAt.After(parsed.IssuedAt.Time) {
		return application.TokenClaims{}, fmt.Errorf("invalid token binding")
	}
	if parsed.TokenPurpose != application.TokenPurposeAccess && parsed.TokenPurpose != application.TokenPurposeRefresh {
		return application.TokenClaims{}, fmt.Errorf("invalid token purpose")
	}
	if parsed.TokenPurpose == application.TokenPurposeRefresh && (parsed.Generation < 1 || !application.ValidSessionIdentifier(parsed.ID)) {
		return application.TokenClaims{}, fmt.Errorf("invalid refresh binding")
	}
	now := time.Now()
	if parsed.IssuedAt.After(now) || parsed.NotBefore != nil && parsed.NotBefore.After(now) {
		return application.TokenClaims{}, fmt.Errorf("token not yet valid")
	}
	expired := !now.Before(parsed.ExpiresAt.Time)
	if expired && (!recoverExpired || parsed.TokenPurpose != application.TokenPurposeRefresh) {
		return application.TokenClaims{}, jwt.ErrTokenExpired
	}
	return application.TokenClaims{UserID: parsed.UserID, TokenVersion: parsed.TokenVersion, Purpose: parsed.TokenPurpose, SessionID: parsed.SessionID, Generation: parsed.Generation, RefreshID: parsed.ID, ExpiresAt: parsed.ExpiresAt.Time, Expired: expired}, nil
}

func (service *Service) sign(value jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, value).SignedString([]byte(service.config.Secret))
}

var _ application.TokenService = (*Service)(nil)
