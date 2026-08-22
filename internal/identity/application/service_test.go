package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin/internal/identity/application"
	"admin/internal/identity/domain"
)

type captchaFake struct {
	verified bool
	calls    int
}

func (fake *captchaFake) Verify(context.Context, string, string) bool {
	fake.calls++
	return fake.verified
}

type userRepositoryFake struct{ user domain.User }

func (fake userRepositoryFake) FindByUsername(context.Context, string) (domain.User, error) {
	return fake.user, nil
}
func (fake userRepositoryFake) FindByID(context.Context, uint) (domain.User, error) {
	return fake.user, nil
}
func (userRepositoryFake) ExistsByUsernameOrEmail(context.Context, string, string) (bool, error) {
	return false, nil
}
func (userRepositoryFake) Create(context.Context, *domain.User) error { return nil }

type passwordFake struct{}

func (passwordFake) Compare(string, string) error { return nil }
func (passwordFake) Hash(string) (string, error)  { return "hash", nil }

type loginAttemptFake struct{ cleared bool }

func (loginAttemptFake) IsLocked(context.Context, string) (time.Duration, bool, error) {
	return 0, false, nil
}
func (loginAttemptFake) RecordFailure(context.Context, string) (int, error) { return 1, nil }
func (fake *loginAttemptFake) Clear(context.Context, string) error          { fake.cleared = true; return nil }
func (loginAttemptFake) Lock(context.Context, string, time.Duration) error  { return nil }

type blacklistFake struct{}

func (blacklistFake) Contains(context.Context, string) (bool, error)   { return false, nil }
func (blacklistFake) Add(context.Context, string, time.Duration) error { return nil }

type authorizationFake struct{}

func (authorizationFake) EnsureVersion(context.Context, uint) (int, error) { return 4, nil }
func (authorizationFake) Snapshot(context.Context, uint) (domain.AccessSnapshot, error) {
	return domain.AccessSnapshot{Roles: []string{"admin"}, Permissions: []string{"admin.users.get"}, Version: 4}, nil
}

type tokenFake struct {
	issue    application.TokenIssue
	claims   application.TokenClaims
	parseErr error
}

func (fake *tokenFake) Issue(_ context.Context, issue application.TokenIssue) (application.TokenPair, error) {
	fake.issue = issue
	return application.TokenPair{AccessToken: "access", RefreshToken: "refresh"}, nil
}
func (fake tokenFake) Parse(context.Context, string) (application.TokenClaims, error) {
	return fake.claims, fake.parseErr
}

type rejectingPasswordFake struct{}

func (rejectingPasswordFake) Compare(string, string) error { return errors.New("wrong password") }
func (rejectingPasswordFake) Hash(string) (string, error)  { return "", nil }

type countingAttemptFake struct {
	failures  int
	lockedFor time.Duration
}

func (countingAttemptFake) IsLocked(context.Context, string) (time.Duration, bool, error) {
	return 0, false, nil
}
func (fake *countingAttemptFake) RecordFailure(context.Context, string) (int, error) {
	fake.failures++
	return fake.failures, nil
}
func (fake *countingAttemptFake) Lock(context.Context, string, time.Duration) error {
	fake.lockedFor = time.Minute
	return nil
}
func (countingAttemptFake) Clear(context.Context, string) error { return nil }

type recordingUserRepository struct {
	userRepositoryFake
	created domain.User
}

func (fake *recordingUserRepository) Create(_ context.Context, user *domain.User) error {
	user.ID = 99
	fake.created = *user
	return nil
}

func TestLoginIssuesTokensFromCurrentAuthorizationSnapshot(t *testing.T) {
	captcha := &captchaFake{verified: true}
	attempts := &loginAttemptFake{}
	tokens := &tokenFake{}
	service := application.NewService(
		userRepositoryFake{user: domain.User{ID: 42, Username: "alice", Password: "stored", Status: 1}},
		captcha,
		passwordFake{},
		attempts,
		blacklistFake{},
		authorizationFake{},
		tokens,
	)

	result, err := service.Login(context.Background(), application.LoginRequest{Username: "alice", Password: "ValidPass123!", CaptchaID: "captcha", CaptchaCode: "123456"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.AccessToken != "access" || result.RefreshToken != "refresh" {
		t.Fatalf("Login() tokens = %#v", result)
	}
	if tokens.issue.UserID != 42 || tokens.issue.TokenVersion != 4 || len(tokens.issue.Roles) != 1 || tokens.issue.Roles[0] != "admin" || tokens.issue.Permissions[0] != "admin.users.get" {
		t.Fatalf("TokenIssue = %#v, want current authorization snapshot", tokens.issue)
	}
	if captcha.calls != 1 || !attempts.cleared {
		t.Fatalf("login dependencies: captcha calls=%d attempts cleared=%v", captcha.calls, attempts.cleared)
	}
}

func TestRegisterValidatesPasswordAndPersistsHashedUser(t *testing.T) {
	repository := &recordingUserRepository{}
	service := application.NewService(repository, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, blacklistFake{}, authorizationFake{}, &tokenFake{})
	user, err := service.Register(context.Background(), application.RegisterRequest{Username: "new-user", Password: "ValidPass123!", Email: "new@example.com", Nickname: "New", CaptchaID: "id", CaptchaCode: "123456"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if user.ID != 99 || repository.created.Password != "hash" || repository.created.Role != "user" || repository.created.Status != 1 {
		t.Fatalf("created user = %#v", repository.created)
	}
}

func TestLoginLocksAfterFifthPasswordFailure(t *testing.T) {
	attempts := &countingAttemptFake{}
	service := application.NewService(userRepositoryFake{user: domain.User{Username: "alice", Password: "stored", Status: 1}}, &captchaFake{verified: true}, rejectingPasswordFake{}, attempts, blacklistFake{}, authorizationFake{}, &tokenFake{})
	var lastErr error
	for i := range 5 {
		_, lastErr = service.Login(context.Background(), application.LoginRequest{Username: "alice", Password: "wrong", CaptchaID: "id", CaptchaCode: "123456"})
		if lastErr == nil {
			t.Fatalf("login attempt %d error = nil", i+1)
		}
	}
	if attempts.failures != 5 || attempts.lockedFor != time.Minute {
		t.Fatalf("lockout state = failures:%d duration:%s", attempts.failures, attempts.lockedFor)
	}
	if code, _ := application.CodeOf(lastErr); code != application.CodeLoginLocked {
		t.Fatalf("fifth failure code = %q, want %q", code, application.CodeLoginLocked)
	}
	details, ok := application.DetailsOf(lastErr)
	if !ok || details.RetryAfterSeconds != 60 {
		t.Fatalf("fifth failure details = %#v, want retry_after_seconds=60", details)
	}
}

func TestPasswordValidationCarriesSafeFieldError(t *testing.T) {
	service := application.NewService(&recordingUserRepository{}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, blacklistFake{}, authorizationFake{}, &tokenFake{})
	_, err := service.Register(context.Background(), application.RegisterRequest{Username: "alice", Password: "weak", CaptchaID: "id", CaptchaCode: "123456"})
	if code, _ := application.CodeOf(err); code != application.CodeValidationInvalid {
		t.Fatalf("validation code = %q, want %q", code, application.CodeValidationInvalid)
	}
	details, ok := application.DetailsOf(err)
	if !ok || len(details.Fields) != 1 || details.Fields[0].Field != "password" || details.Fields[0].ErrorCode != "IDENTITY_PASSWORD_INVALID" {
		t.Fatalf("validation details = %#v", details)
	}
}

func TestRefreshUsesCurrentAuthorizationSnapshot(t *testing.T) {
	tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: 4, Purpose: application.TokenPurposeRefresh}}
	service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, blacklistFake{}, authorizationFake{}, tokens)
	pair, err := service.Refresh(context.Background(), application.RefreshRequest{RefreshToken: "refresh"})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if pair.AccessToken != "access" || tokens.issue.UserID != 42 || tokens.issue.TokenVersion != 4 || tokens.issue.Permissions[0] != "admin.users.get" {
		t.Fatalf("refresh issue = %#v pair=%#v", tokens.issue, pair)
	}
}

func TestRefreshRejectsStaleAuthorizationVersion(t *testing.T) {
	tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: 3, Purpose: application.TokenPurposeRefresh}}
	service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, blacklistFake{}, authorizationFake{}, tokens)
	if _, err := service.Refresh(context.Background(), application.RefreshRequest{RefreshToken: "refresh"}); !errors.Is(err, application.ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestAuthenticateUsesRuntimeAuthorizationAndRejectsRefreshPurpose(t *testing.T) {
	tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: 4, Purpose: application.TokenPurposeAccess}}
	service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, blacklistFake{}, authorizationFake{}, tokens)
	identity, err := service.Authenticate(context.Background(), "access")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if identity.UserID != 42 || identity.Roles[0] != "admin" || identity.Permissions[0] != "admin.users.get" {
		t.Fatalf("authenticated identity = %#v", identity)
	}
	tokens.claims.Purpose = application.TokenPurposeRefresh
	if _, err := service.Authenticate(context.Background(), "refresh"); err == nil {
		t.Fatal("Authenticate() accepted refresh token")
	}
}
