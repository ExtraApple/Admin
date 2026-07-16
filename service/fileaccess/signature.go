package fileaccess

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModeDownload Mode = "download"
	ModePreview  Mode = "preview"
)

var (
	ErrInvalidSignature = errors.New("invalid file access signature")
	ErrExpired          = errors.New("file access signature expired")
)

// Claims contains every mutable access decision bound to a temporary file URL.
type Claims struct {
	UserID           uint
	FileID           uint
	Mode             Mode
	ExpiresAt        int64
	ValidationStatus string
}

// Signer creates and verifies URL-safe HMAC-SHA256 signatures.
type Signer struct {
	key []byte
}

func NewSigner(key []byte) (*Signer, error) {
	if len(key) == 0 {
		return nil, errors.New("file access signing key is empty")
	}
	return &Signer{key: append([]byte(nil), key...)}, nil
}

func (s *Signer) Sign(claims Claims) (string, error) {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(canonicalClaims(claims))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Signer) Verify(claims Claims, signature string, now time.Time) error {
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
		return ErrExpired
	}
	return nil
}

func canonicalClaims(claims Claims) []byte {
	return []byte(strings.Join([]string{
		"v1",
		strconv.FormatUint(uint64(claims.UserID), 10),
		strconv.FormatUint(uint64(claims.FileID), 10),
		string(claims.Mode),
		strconv.FormatInt(claims.ExpiresAt, 10),
		claims.ValidationStatus,
	}, "\n"))
}
