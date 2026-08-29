package application

import (
	"context"
	"time"

	"admin/internal/identity/domain"
)

type UserRepository interface {
	FindByUsername(context.Context, string) (domain.User, error)
	FindByID(context.Context, uint) (domain.User, error)
	ExistsByUsernameOrEmail(context.Context, string, string) (bool, error)
	Create(context.Context, *domain.User) error
}

// DirectoryUserRecord is the narrow persistence result used only by the
// Identity directory application service. It contains no object key, legacy
// avatar URL, ORM model, or storage client.
type DirectoryUserRecord struct {
	ID            uint
	Username      string
	Nickname      string
	Email         string
	Role          string
	Status        int
	AvatarTrusted bool
}

// DirectoryRepository supplies only the fields needed to build public user
// directory values and the user IDs needed by cross-module invalidation.
type DirectoryRepository interface {
	ListUsersByIDs(context.Context, []uint) ([]DirectoryUserRecord, error)
	ListUserIDs(context.Context) ([]uint, error)
}

// UserDirectory is the Identity-owned user read capability consumed by other
// modules. Implementations return only safe, public directory values.
type UserDirectory interface {
	ListUsersByIDs(context.Context, []uint) ([]domain.DirectoryUser, error)
	ListUserIDs(context.Context) ([]uint, error)
}

type UserChanges struct {
	Nickname             *string
	Email                *string
	PendingEmail         *string
	ClearEmailVerifiedAt bool
	Role                 *string
	Status               *int
}

type UserManagementRepository interface {
	UserRepository
	List(context.Context, int, int, domain.UserScope) ([]domain.User, int64, error)
	EmailExists(context.Context, string, uint) (bool, error)
	Update(context.Context, uint, UserChanges) error
	UpdatePassword(context.Context, uint, string) error
	Delete(context.Context, uint) error
}

type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}

type AccessManager interface {
	UserScope(context.Context, uint) (domain.UserScope, error)
	IncrementVersions(context.Context, []uint) error
	IsAdministrator(context.Context, uint) (bool, error)
}

type CaptchaVerifier interface {
	Verify(context.Context, string, string) bool
}

type PasswordHasher interface {
	Compare(string, string) error
	Hash(string) (string, error)
}

type LoginAttemptStore interface {
	IsLocked(context.Context, string) (time.Duration, bool, error)
	RecordFailure(context.Context, string) (int, error)
	Lock(context.Context, string, time.Duration) error
	Clear(context.Context, string) error
}

type BlacklistStore interface {
	Contains(context.Context, string) (bool, error)
	Add(context.Context, string, time.Duration) error
}

type AuthorizationReader interface {
	EnsureVersion(context.Context, uint) (int, error)
	Snapshot(context.Context, uint) (domain.AccessSnapshot, error)
}

type TokenService interface {
	Issue(context.Context, TokenIssue) (TokenPair, error)
	Parse(context.Context, string) (TokenClaims, error)
}

// NavigationReader supplies only the user-visible menu value tree.
type NavigationReader interface {
	UserMenus(context.Context, uint) ([]domain.Menu, error)
}

// OrganizationReader supplies only organization membership facts needed by
// Identity user-management policies; persistence types remain private.
type OrganizationReader interface {
	MemberOrganizationIDs(context.Context, uint) ([]uint, error)
	ExistingUserIDs(context.Context, []uint) ([]uint, error)
}
type EmailVerificationConfirmation struct {
	Conflict bool
}

type EmailVerificationRepository interface {
	CountIssuedSince(context.Context, uint, time.Time) (int, error)
	ReplaceActive(context.Context, domain.EmailVerificationCredential) error
	InvalidateActive(context.Context, uint, string, time.Time) error
	Confirm(context.Context, uint, string, time.Time) (EmailVerificationConfirmation, error)
	DeleteTerminalBefore(context.Context, time.Time) error
}

type EmailVerificationIssuer interface {
	Issue(context.Context, uint, string) (string, error)
	Invalidate(context.Context, uint, string) error
}

type VerificationEmailSender interface {
	SendVerification(context.Context, string, string) error
}
