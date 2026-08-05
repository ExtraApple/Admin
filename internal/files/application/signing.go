package application

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

const signingKeyPurpose = "admin:file-access-signing:v1"

var (
	ErrInvalidSignature = errors.New("invalid file access signature")
	ErrExpiredSignature = errors.New("file access signature expired")
)

type HMACSigner struct{ key []byte }

func NewHMACSigner(key []byte) (*HMACSigner, error) {
	if len(key) == 0 {
		return nil, errors.New("file access signing key is empty")
	}
	return &HMACSigner{key: append([]byte(nil), key...)}, nil
}
func NewHMACSignerFromJWTSecret(secret string) (*HMACSigner, error) {
	if secret == "" {
		return nil, errors.New("JWT secret is empty")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingKeyPurpose))
	return NewHMACSigner(mac.Sum(nil))
}
func (s *HMACSigner) Sign(claims Claims) (string, error) {
	if s == nil || len(s.key) == 0 {
		return "", errors.New("file access signing is not configured")
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(canonicalClaims(claims))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (s *HMACSigner) Verify(claims Claims, signature string, now time.Time) error {
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(canonicalClaims(claims))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return ErrInvalidSignature
	}
	if !now.Before(time.Unix(claims.ExpiresAt, 0)) {
		return ErrExpiredSignature
	}
	return nil
}
func canonicalClaims(claims Claims) []byte {
	return []byte(strings.Join([]string{"v1", strconv.FormatUint(uint64(claims.UserID), 10), strconv.FormatUint(uint64(claims.FileID), 10), string(claims.Mode), strconv.FormatInt(claims.ExpiresAt, 10), claims.ValidationStatus}, "\n"))
}

var _ Signer = (*HMACSigner)(nil)
