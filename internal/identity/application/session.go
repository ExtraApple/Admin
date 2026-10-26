package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"admin/internal/identity/domain"
)

var (
	ErrSessionMissing     = errors.New("login session missing or revoked")
	ErrRefreshUncommitted = errors.New("refresh attempt has no committed result")
	ErrRefreshReuse       = errors.New("refresh credential reused")
	ErrRefreshSuperseded  = NewError(CodeRefreshSuperseded, nil)
)

const RefreshRecoveryWindow = 30 * time.Second

// SessionStore owns all linearized login and refresh state transitions. Missing
// includes revoked/expired terminal facts; other errors are infrastructure errors.
type SessionStore interface {
	Create(context.Context, domain.LoginSession, *TokenClaims) error
	Read(context.Context, string) (domain.LoginSession, error)
	Rotate(context.Context, RefreshRotation) (TokenIssue, error)
	Revoke(context.Context, TokenClaims) error
}

// RefreshRotation carries a verified credential and optional fixed candidate.
// With no candidate, Rotate returns an existing result or ErrRefreshUncommitted
// after atomically checking a still-valid current credential. This probe never
// advances a generation or its TTL; all rejection/reuse checks still apply.
type RefreshRotation struct {
	Credential TokenClaims
	AttemptID  string
	Candidate  TokenIssue
}

var sessionIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

func ValidSessionIdentifier(value string) bool { return sessionIdentifier.MatchString(value) }

func CredentialDigest(identifier string) string {
	digest := sha256.Sum256([]byte(identifier))
	return hex.EncodeToString(digest[:])
}

func SessionFromIssue(issue TokenIssue) domain.LoginSession {
	return domain.LoginSession{ID: issue.SessionID, UserID: issue.UserID, TokenVersion: issue.TokenVersion, Generation: issue.Generation, RefreshDigest: CredentialDigest(issue.RefreshID), ExpiresAt: issue.RefreshExpiresAt}
}
