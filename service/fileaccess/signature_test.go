package fileaccess

import (
	"errors"
	"testing"
	"time"
)

func TestSignerMatchesKnownHMACSHA256Vector(t *testing.T) {
	signer, err := NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	claims := Claims{
		UserID:           42,
		FileID:           99,
		Mode:             ModeDownload,
		ExpiresAt:        1_700_000_300,
		ValidationStatus: "validated",
	}

	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	const want = "CEo77lAFV6s0eqxDk3UeaztEcRa0614-W8zF9yzFzPk"
	if signature != want {
		t.Fatalf("Sign() = %q, want known HMAC vector %q", signature, want)
	}

	if err := signer.Verify(claims, signature, time.Unix(1_700_000_200, 0)); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestSignerRejectsExpiredClaims(t *testing.T) {
	signer, err := NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	claims := Claims{
		UserID:           42,
		FileID:           99,
		Mode:             ModeDownload,
		ExpiresAt:        1_700_000_300,
		ValidationStatus: "validated",
	}
	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	err = signer.Verify(claims, signature, time.Unix(claims.ExpiresAt, 0))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Verify() error = %v, want ErrExpired", err)
	}
}

func TestSignerRejectsMalformedOrIncorrectSignature(t *testing.T) {
	signer, err := NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}
	claims := Claims{
		UserID:           42,
		FileID:           99,
		Mode:             ModePreview,
		ExpiresAt:        1_700_000_300,
		ValidationStatus: "validated",
	}

	for _, signature := range []string{
		"not-base64!",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		err := signer.Verify(claims, signature, time.Unix(1_700_000_200, 0))
		if !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("Verify(%q) error = %v, want ErrInvalidSignature", signature, err)
		}
	}
}
