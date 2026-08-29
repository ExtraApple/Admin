package application

import (
	"context"
	"errors"
	"time"
	"unicode"

	"admin/internal/identity/domain"
)

const maxLoginFailures = 5

var (
	ErrCaptchaInvalid         = NewError(CodeCaptchaInvalid, nil)
	ErrRefreshTokenInvalid    = NewError(CodeRefreshTokenInvalid, nil)
	ErrAvatarFieldNotWritable = NewError(CodeValidationInvalid, nil)
	ErrUserNotFound           = NewError(CodeUserNotFound, nil)
)

type LoginRequest struct {
	Username    string
	Password    string
	CaptchaID   string
	CaptchaCode string
}

type RegisterRequest struct {
	Username    string
	Password    string
	Email       string
	Nickname    string
	CaptchaID   string
	CaptchaCode string
}

type RefreshRequest struct{ RefreshToken string }

type TokenIssue struct {
	UserID       uint
	TokenVersion int
	Roles        []string
	Permissions  []string
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type TokenPurpose string

const (
	TokenPurposeAccess  TokenPurpose = "access"
	TokenPurposeRefresh TokenPurpose = "refresh"
)

type TokenClaims struct {
	UserID       uint
	TokenVersion int
	Purpose      TokenPurpose
}

type AuthenticatedIdentity struct {
	UserID      uint
	Roles       []string
	Permissions []string
}
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	User         domain.User
}

// Service owns authentication and token lifecycle use cases without exposing
// persistence, Redis, JWT, or HTTP implementation details.
type Service struct {
	users                   UserRepository
	captcha                 CaptchaVerifier
	passwords               PasswordHasher
	attempts                LoginAttemptStore
	blacklist               BlacklistStore
	authorization           AuthorizationReader
	tokens                  TokenService
	emailVerificationIssuer EmailVerificationIssuer
	verificationEmailSender VerificationEmailSender
}

func NewService(
	users UserRepository,
	captcha CaptchaVerifier,
	passwords PasswordHasher,
	attempts LoginAttemptStore,
	blacklist BlacklistStore,
	authorization AuthorizationReader,
	tokens TokenService,
) *Service {
	return &Service{users: users, captcha: captcha, passwords: passwords, attempts: attempts, blacklist: blacklist, authorization: authorization, tokens: tokens}
}

func (service *Service) ConfigureEmailVerification(issuer EmailVerificationIssuer, sender VerificationEmailSender) {
	service.emailVerificationIssuer = issuer
	service.verificationEmailSender = sender
}

func (service *Service) Register(ctx context.Context, request RegisterRequest) (domain.User, error) {
	if !service.captcha.Verify(ctx, request.CaptchaID, request.CaptchaCode) {
		return domain.User{}, ErrCaptchaInvalid
	}
	if err := validatePassword(request.Password); err != nil {
		return domain.User{}, err
	}
	exists, err := service.users.ExistsByUsernameOrEmail(ctx, request.Username, request.Email)
	if err != nil {
		return domain.User{}, NewError(CodeInternalError, err)
	}
	if exists {
		return domain.User{}, NewError(CodeConflict, nil)
	}
	hashed, err := service.passwords.Hash(request.Password)
	if err != nil {
		return domain.User{}, NewError(CodeInternalError, err)
	}
	user := domain.User{Username: request.Username, Password: hashed, Email: request.Email, Nickname: request.Nickname, Role: "user", Status: 1}
	if err := service.users.Create(ctx, &user); err != nil {
		return domain.User{}, NewError(CodeInternalError, err)
	}
	if service.emailVerificationIssuer != nil && service.verificationEmailSender != nil {
		token, err := service.emailVerificationIssuer.Issue(ctx, user.ID, user.Email)
		if err != nil {
			return user, err
		}
		if err := service.verificationEmailSender.SendVerification(ctx, user.Email, token); err != nil {
			_ = service.emailVerificationIssuer.Invalidate(ctx, user.ID, user.Email)
			return user, NewError(CodeEmailVerificationDeliveryFailed, err)
		}
	}
	return user, nil
}

func (service *Service) Login(ctx context.Context, request LoginRequest) (LoginResult, error) {
	if !service.captcha.Verify(ctx, request.CaptchaID, request.CaptchaCode) {
		return LoginResult{}, ErrCaptchaInvalid
	}
	if remaining, locked, err := service.attempts.IsLocked(ctx, request.Username); err != nil {
		return LoginResult{}, NewError(CodeInternalError, err)
	} else if locked {
		return LoginResult{}, NewLoginLockedError(retryAfterSeconds(remaining), nil)
	}
	user, err := service.users.FindByUsername(ctx, request.Username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return LoginResult{}, NewCredentialsError(0, nil)
		}
		return LoginResult{}, NewError(CodeInternalError, err)
	}
	if !user.Enabled() {
		return LoginResult{}, NewError(CodeAccountDisabled, nil)
	}
	if err := service.passwords.Compare(user.Password, request.Password); err != nil {
		failures, recordErr := service.attempts.RecordFailure(ctx, request.Username)
		if recordErr != nil {
			return LoginResult{}, NewError(CodeInternalError, recordErr)
		}
		if lockDuration := lockDurationForFailures(failures); lockDuration > 0 {
			if err := service.attempts.Lock(ctx, request.Username, lockDuration); err != nil {
				return LoginResult{}, NewError(CodeInternalError, err)
			}
			return LoginResult{}, NewLoginLockedError(retryAfterSeconds(lockDuration), nil)
		}
		return LoginResult{}, NewCredentialsError(maxLoginFailures-failures, nil)
	}

	version, err := service.authorization.EnsureVersion(ctx, user.ID)
	if err != nil {
		return LoginResult{}, NewError(CodeInternalError, err)
	}
	snapshot, err := service.authorization.Snapshot(ctx, user.ID)
	if err != nil {
		return LoginResult{}, NewError(CodeInternalError, err)
	}
	if snapshot.Version != version {
		return LoginResult{}, NewError(CodeInternalError, errors.New("authorization version changed during login"))
	}
	tokens, err := service.tokens.Issue(ctx, TokenIssue{UserID: user.ID, TokenVersion: version, Roles: cloneStrings(snapshot.Roles), Permissions: cloneStrings(snapshot.Permissions)})
	if err != nil {
		return LoginResult{}, NewError(CodeInternalError, err)
	}
	if err := service.attempts.Clear(ctx, request.Username); err != nil {
		return LoginResult{}, NewError(CodeInternalError, err)
	}
	return LoginResult{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, User: user}, nil
}

func (service *Service) Refresh(ctx context.Context, request RefreshRequest) (TokenPair, error) {
	claims, err := service.tokens.Parse(ctx, request.RefreshToken)
	if err != nil || claims.Purpose != TokenPurposeRefresh {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	blacklisted, err := service.blacklist.Contains(ctx, request.RefreshToken)
	if err != nil || blacklisted {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	user, err := service.users.FindByID(ctx, claims.UserID)
	if err != nil || !user.Enabled() {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	snapshot, err := service.authorization.Snapshot(ctx, claims.UserID)
	if err != nil || claims.TokenVersion <= 0 || claims.TokenVersion != snapshot.Version {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	pair, err := service.tokens.Issue(ctx, TokenIssue{UserID: claims.UserID, TokenVersion: snapshot.Version, Roles: cloneStrings(snapshot.Roles), Permissions: cloneStrings(snapshot.Permissions)})
	if err != nil {
		return TokenPair{}, NewError(CodeInternalError, err)
	}
	return pair, nil
}

func (service *Service) Authenticate(ctx context.Context, token string) (AuthenticatedIdentity, error) {
	claims, err := service.tokens.Parse(ctx, token)
	if err != nil || claims.Purpose != TokenPurposeAccess {
		return AuthenticatedIdentity{}, NewError(CodeTokenInvalid, err)
	}
	blacklisted, err := service.blacklist.Contains(ctx, token)
	if err != nil {
		return AuthenticatedIdentity{}, NewError(CodeInternalError, err)
	}
	if blacklisted {
		return AuthenticatedIdentity{}, NewError(CodeTokenInvalid, nil)
	}
	user, err := service.users.FindByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return AuthenticatedIdentity{}, NewError(CodeTokenInvalid, err)
		}
		return AuthenticatedIdentity{}, NewError(CodeInternalError, err)
	}
	if !user.Enabled() {
		return AuthenticatedIdentity{}, NewError(CodeAccountDisabled, nil)
	}
	snapshot, err := service.authorization.Snapshot(ctx, claims.UserID)
	if err != nil {
		return AuthenticatedIdentity{}, NewError(CodeInternalError, err)
	}
	if claims.TokenVersion <= 0 || claims.TokenVersion != snapshot.Version {
		return AuthenticatedIdentity{}, NewError(CodeTokenInvalid, nil)
	}
	return AuthenticatedIdentity{UserID: claims.UserID, Roles: cloneStrings(snapshot.Roles), Permissions: cloneStrings(snapshot.Permissions)}, nil
}

func (service *Service) Logout(ctx context.Context, token string, expire time.Duration) error {
	if err := service.blacklist.Add(ctx, token, expire); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

func lockDurationForFailures(failures int) time.Duration {
	switch {
	case failures < maxLoginFailures:
		return 0
	case failures < 10:
		return time.Minute
	case failures < 15:
		return 5 * time.Minute
	case failures < 20:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

func retryAfterSeconds(duration time.Duration) int {
	seconds := int((duration + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func validatePassword(password string) error {
	if len(password) < 6 {
		return NewValidationError([]FieldError{{Field: "password", ErrorCode: "IDENTITY_PASSWORD_INVALID", Message: "password does not meet the security requirements"}}, nil)
	}
	var upper, lower, digit, special bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsLower(char):
			lower = true
		case unicode.IsDigit(char):
			digit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			special = true
		}
	}
	count := 0
	for _, present := range []bool{upper, lower, digit, special} {
		if present {
			count++
		}
	}
	if count < 3 {
		return NewValidationError([]FieldError{{Field: "password", ErrorCode: "IDENTITY_PASSWORD_INVALID", Message: "password does not meet the security requirements"}}, nil)
	}
	return nil
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }
