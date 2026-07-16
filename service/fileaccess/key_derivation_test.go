package fileaccess

import (
	"errors"
	"testing"
	"time"
)

func TestSignerDerivedFromJWTSecretMatchesFixedPurposeVector(t *testing.T) {
	signer, err := NewSignerFromJWTSecret("jwt-secret-for-tests")
	if err != nil {
		t.Fatalf("NewSignerFromJWTSecret() error = %v", err)
	}
	claims := testClaims()

	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	const want = "zrUeRL048DxX8NH0X_Lq23MOp7L4Bs2Nd5d8jqdk9Bk"
	if signature != want {
		t.Fatalf("Sign() = %q, want fixed-purpose derivation vector %q", signature, want)
	}
}

func TestDerivedSignerRejectsEveryBoundClaimTamper(t *testing.T) {
	signer, err := NewSignerFromJWTSecret("jwt-secret-for-tests")
	if err != nil {
		t.Fatalf("NewSignerFromJWTSecret() error = %v", err)
	}
	claims := testClaims()
	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	now := time.Unix(1_700_000_200, 0)

	tests := []struct {
		name   string
		mutate func(*Claims)
	}{
		{"user", func(value *Claims) { value.UserID++ }},
		{"file", func(value *Claims) { value.FileID++ }},
		{"mode", func(value *Claims) { value.Mode = ModePreview }},
		{"expires", func(value *Claims) { value.ExpiresAt++ }},
		{"status", func(value *Claims) { value.ValidationStatus = "blocked" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tampered := claims
			tt.mutate(&tampered)
			err := signer.Verify(tampered, signature, now)
			if !errors.Is(err, ErrInvalidSignature) {
				t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
			}
		})
	}

	t.Run("signature", func(t *testing.T) {
		tampered := "A" + signature[1:]
		err := signer.Verify(claims, tampered, now)
		if !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("Verify() error = %v, want ErrInvalidSignature", err)
		}
	})
}

func TestDerivedSignerRejectsDifferentJWTSecretAndExpiredAccess(t *testing.T) {
	signer, err := NewSignerFromJWTSecret("jwt-secret-for-tests")
	if err != nil {
		t.Fatalf("NewSignerFromJWTSecret() error = %v", err)
	}
	otherSigner, err := NewSignerFromJWTSecret("different-jwt-secret")
	if err != nil {
		t.Fatalf("NewSignerFromJWTSecret(other) error = %v", err)
	}
	claims := testClaims()
	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	if err := otherSigner.Verify(claims, signature, time.Unix(1_700_000_200, 0)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify() with different JWT secret error = %v, want ErrInvalidSignature", err)
	}
	if err := signer.Verify(claims, signature, time.Unix(claims.ExpiresAt, 0)); !errors.Is(err, ErrExpired) {
		t.Fatalf("Verify() at expiry error = %v, want ErrExpired", err)
	}
}

func testClaims() Claims {
	return Claims{
		UserID:           42,
		FileID:           99,
		Mode:             ModeDownload,
		ExpiresAt:        1_700_000_300,
		ValidationStatus: "validated",
	}
}
