package fileaccess

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
)

const signingKeyPurpose = "admin:file-access-signing:v1"

// NewSignerFromJWTSecret derives an independent file-access signing key from
// the JWT secret. The JWT secret itself is never used directly as an HMAC key
// for temporary file URLs.
func NewSignerFromJWTSecret(jwtSecret string) (*Signer, error) {
	if jwtSecret == "" {
		return nil, errors.New("JWT secret is empty")
	}

	mac := hmac.New(sha256.New, []byte(jwtSecret))
	_, _ = mac.Write([]byte(signingKeyPurpose))
	return NewSigner(mac.Sum(nil))
}
