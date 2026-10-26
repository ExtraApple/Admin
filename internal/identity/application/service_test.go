package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	jwtadapter "admin/internal/identity/adapters/jwt"
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

type sessionResult struct {
	material application.TokenIssue
	previous string
	deadline time.Time
}
type sessionFake struct {
	sessions  map[string]domain.LoginSession
	results   map[string]sessionResult
	readErr   error
	createErr error
	revokeErr error
	rotateErr error
}

func newSessionFake() *sessionFake {
	return &sessionFake{sessions: map[string]domain.LoginSession{"test-session-00000001": {ID: "test-session-00000001", UserID: 42, TokenVersion: 4, Generation: 1, RefreshDigest: application.CredentialDigest("test-refresh-00000001"), ExpiresAt: time.Now().Add(time.Hour)}}, results: make(map[string]sessionResult)}
}

func (fake *sessionFake) Create(_ context.Context, session domain.LoginSession, previous *application.TokenClaims) error {
	if fake.createErr != nil {
		return fake.createErr
	}
	fake.sessions[session.ID] = session
	if previous != nil {
		delete(fake.sessions, previous.SessionID)
	}
	return nil
}
func (fake *sessionFake) Read(_ context.Context, id string) (domain.LoginSession, error) {
	if fake.readErr != nil {
		return domain.LoginSession{}, fake.readErr
	}
	session, ok := fake.sessions[id]
	if !ok || !session.ExpiresAt.After(time.Now()) {
		return domain.LoginSession{}, application.ErrSessionMissing
	}
	return session, nil
}
func (fake *sessionFake) Rotate(ctx context.Context, request application.RefreshRotation) (application.TokenIssue, error) {
	if fake.rotateErr != nil {
		return application.TokenIssue{}, fake.rotateErr
	}
	session, err := fake.Read(ctx, request.Credential.SessionID)
	if err != nil {
		return application.TokenIssue{}, err
	}
	if session.UserID != request.Credential.UserID || session.TokenVersion != request.Credential.TokenVersion {
		return application.TokenIssue{}, application.ErrSessionMissing
	}
	key := session.ID + ":" + request.AttemptID
	if result, ok := fake.results[key]; ok && time.Now().Before(result.deadline) {
		nextDigest := application.CredentialDigest(result.material.RefreshID)
		bound := request.Credential.RefreshID == result.previous && request.Credential.Generation == result.material.Generation-1 || application.CredentialDigest(request.Credential.RefreshID) == nextDigest && request.Credential.Generation == result.material.Generation
		if bound && session.Generation > result.material.Generation {
			return application.TokenIssue{}, application.ErrRefreshSuperseded
		}
		if bound && session.Generation == result.material.Generation && session.RefreshDigest == nextDigest {
			return result.material, nil
		}
		delete(fake.sessions, session.ID)
		return application.TokenIssue{}, application.ErrRefreshReuse
	}
	if session.Generation != request.Credential.Generation || session.RefreshDigest != application.CredentialDigest(request.Credential.RefreshID) {
		delete(fake.sessions, session.ID)
		return application.TokenIssue{}, application.ErrRefreshReuse
	}
	if request.Credential.Expired {
		return application.TokenIssue{}, application.ErrSessionMissing
	}
	if request.Candidate.SessionID == "" {
		return application.TokenIssue{}, application.ErrRefreshUncommitted
	}
	fake.sessions[session.ID] = application.SessionFromIssue(request.Candidate)
	fake.results[key] = sessionResult{material: request.Candidate, previous: request.Credential.RefreshID, deadline: time.Now().Add(application.RefreshRecoveryWindow)}
	return request.Candidate, nil
}
func (fake *sessionFake) Revoke(_ context.Context, claims application.TokenClaims) error {
	if fake.revokeErr != nil {
		return fake.revokeErr
	}
	delete(fake.sessions, claims.SessionID)
	return nil
}

type authorizationFake struct{}

func (authorizationFake) EnsureVersion(context.Context, uint) (int, error) { return 4, nil }
func (authorizationFake) Snapshot(context.Context, uint) (domain.AccessSnapshot, error) {
	return domain.AccessSnapshot{Roles: []string{"admin"}, Permissions: []string{"admin.users.get"}, Version: 4}, nil
}

type tokenFake struct {
	issue        application.TokenIssue
	claims       application.TokenClaims
	parseErr     error
	prepareErr   error
	issueErr     error
	prepareCalls int
	issueCalls   int
}

func (fake *tokenFake) Prepare(ctx context.Context, issue application.TokenIssue) (application.TokenIssue, error) {
	fake.prepareCalls++
	if fake.prepareErr != nil {
		return application.TokenIssue{}, fake.prepareErr
	}
	return jwtadapter.NewService(jwtadapter.Config{Secret: "fixture-signing-secret", AccessExpireMins: 15, RefreshExpireMins: 60}).Prepare(ctx, issue)
}

func (fake *tokenFake) Issue(ctx context.Context, issue application.TokenIssue) (application.TokenPair, error) {
	fake.issueCalls++
	if fake.issueErr != nil {
		return application.TokenPair{}, fake.issueErr
	}
	pair, err := jwtadapter.NewService(jwtadapter.Config{Secret: "fixture-signing-secret", AccessExpireMins: 15, RefreshExpireMins: 60}).Issue(ctx, issue)
	fake.issue = pair.Material
	return pair, err
}
func (fake tokenFake) Parse(ctx context.Context, token string) (application.TokenClaims, error) {
	if fake.claims.UserID != 0 || fake.parseErr != nil {
		return fake.claims, fake.parseErr
	}
	return jwtadapter.NewService(jwtadapter.Config{Secret: "fixture-signing-secret", AccessExpireMins: 15, RefreshExpireMins: 60}).Parse(ctx, token)
}
func (fake tokenFake) ParseRefresh(ctx context.Context, token string) (application.TokenClaims, error) {
	if fake.claims.UserID != 0 || fake.parseErr != nil {
		return fake.claims, fake.parseErr
	}
	return jwtadapter.NewService(jwtadapter.Config{Secret: "fixture-signing-secret", AccessExpireMins: 15, RefreshExpireMins: 60}).ParseRefresh(ctx, token)
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
		newSessionFake(),
		authorizationFake{},
		tokens,
	)

	result, err := service.Login(context.Background(), application.LoginRequest{Username: "alice", Password: "ValidPass123!", CaptchaID: "captcha", CaptchaCode: "123456"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" || !application.ValidSessionIdentifier(result.SessionID) {
		t.Fatal("login omitted signed credentials or session binding")
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
	service := application.NewService(repository, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, newSessionFake(), authorizationFake{}, &tokenFake{})
	user, err := service.Register(context.Background(), application.RegisterRequest{Username: "new-user", Password: "ValidPass123!", Email: "new@example.com", Nickname: "New", CaptchaID: "id", CaptchaCode: "123456"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if user.ID != 99 || repository.created.Password != "hash" || repository.created.Role != "user" || repository.created.Status != 1 {
		t.Fatalf("created user = %#v", repository.created)
	}
}

type missingUserRepositoryFake struct{ userRepositoryFake }

func (missingUserRepositoryFake) FindByUsername(context.Context, string) (domain.User, error) {
	return domain.User{}, application.ErrUserNotFound
}

func TestLoginCredentialFailuresAreIndistinguishableThroughLockout(t *testing.T) {
	for _, test := range []struct {
		name  string
		users application.UserRepository
	}{
		{"wrong password", userRepositoryFake{user: domain.User{Username: "alice", Password: "stored", Status: 1}}},
		{"missing username", missingUserRepositoryFake{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts := &countingAttemptFake{}
			service := application.NewService(test.users, &captchaFake{verified: true}, rejectingPasswordFake{}, attempts, newSessionFake(), authorizationFake{}, &tokenFake{})
			for attempt := 1; attempt <= 5; attempt++ {
				result, err := service.Login(context.Background(), application.LoginRequest{Username: "alice", Password: "wrong", CaptchaID: "id", CaptchaCode: "123456"})
				code, _ := application.CodeOf(err)
				details, ok := application.DetailsOf(err)
				if result.AccessToken != "" || result.RefreshToken != "" || !ok || attempts.failures != attempt {
					t.Fatalf("attempt %d: result=%#v details=%#v failures=%d", attempt, result, details, attempts.failures)
				}
				if attempt < 5 {
					if code != application.CodeCredentialsInvalid || details.RemainingAttempts != 5-attempt || details.RetryAfterSeconds != 0 || attempts.lockedFor != 0 {
						t.Fatalf("attempt %d: code=%q details=%#v locked=%s", attempt, code, details, attempts.lockedFor)
					}
				} else if code != application.CodeLoginLocked || details.RetryAfterSeconds != 60 || attempts.lockedFor != time.Minute {
					t.Fatalf("fifth failure: code=%q details=%#v locked=%s", code, details, attempts.lockedFor)
				}
			}
		})
	}
}

func TestPasswordValidationCarriesSafeFieldError(t *testing.T) {
	service := application.NewService(&recordingUserRepository{}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, newSessionFake(), authorizationFake{}, &tokenFake{})
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
	tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: 4, Purpose: application.TokenPurposeRefresh, SessionID: "test-session-00000001", Generation: 1, RefreshID: "test-refresh-00000001", ExpiresAt: time.Now().Add(time.Hour)}}
	service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, newSessionFake(), authorizationFake{}, tokens)
	pair, err := service.Refresh(context.Background(), application.RefreshRequest{RefreshToken: "refresh", AttemptID: "test-attempt-00000001"})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if pair.AccessToken == "" || tokens.issue.UserID != 42 || tokens.issue.TokenVersion != 4 || tokens.issue.Permissions[0] != "admin.users.get" {
		t.Fatal("refresh did not use the current authorization snapshot")
	}
}

func TestRefreshRejectsStaleAuthorizationVersion(t *testing.T) {
	tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: 3, Purpose: application.TokenPurposeRefresh, SessionID: "test-session-00000001", Generation: 1, RefreshID: "test-refresh-00000001", ExpiresAt: time.Now().Add(time.Hour)}}
	service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, newSessionFake(), authorizationFake{}, tokens)
	if _, err := service.Refresh(context.Background(), application.RefreshRequest{RefreshToken: "refresh", AttemptID: "test-attempt-00000001"}); !errors.Is(err, application.ErrRefreshTokenInvalid) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshTokenInvalid", err)
	}
}

func TestAuthenticateUsesRuntimeAuthorizationAndRejectsRefreshPurpose(t *testing.T) {
	tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: 4, Purpose: application.TokenPurposeAccess, SessionID: "test-session-00000001", Generation: 1, RefreshID: "test-refresh-00000001", ExpiresAt: time.Now().Add(time.Hour)}}
	service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, newSessionFake(), authorizationFake{}, tokens)
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

type failingAuthorization struct {
	authorizationFake
	cause error
}

func (fake failingAuthorization) Snapshot(context.Context, uint) (domain.AccessSnapshot, error) {
	return domain.AccessSnapshot{}, fake.cause
}

func TestAuthenticateSeparatesMissingAuthorizationVersionFromInfrastructureFailure(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		code  application.ErrorCode
	}{
		{"missing version", errors.Join(errors.New("snapshot failed"), application.ErrAuthorizationVersionMissing), application.CodeTokenInvalid},
		{"infrastructure failure", errors.New("version query failed"), application.CodeInternalError},
		{"same text without error identity", errors.New(application.ErrAuthorizationVersionMissing.Error()), application.CodeInternalError},
	} {
		t.Run(test.name, func(t *testing.T) {
			tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: 4, Purpose: application.TokenPurposeAccess, SessionID: "test-session-00000001", Generation: 1, RefreshID: "test-refresh-00000001", ExpiresAt: time.Now().Add(time.Hour)}}
			service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, newSessionFake(), failingAuthorization{cause: test.cause}, tokens)
			identity, err := service.Authenticate(context.Background(), "access")
			code, _ := application.CodeOf(err)
			if code != test.code || identity.UserID != 0 || !errors.Is(err, test.cause) {
				t.Fatalf("Authenticate() = %#v, %v; code=%q want %q with original cause", identity, err, code, test.code)
			}
		})
	}
}
func TestAuthenticateRejectsDisabledAndStaleAccess(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   int
		tokenVer int
		wantCode application.ErrorCode
	}{
		{"disabled account", 0, 4, application.CodeAccountDisabled},
		{"stale access version", 1, 3, application.CodeTokenInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			tokens := &tokenFake{claims: application.TokenClaims{UserID: 42, TokenVersion: test.tokenVer, Purpose: application.TokenPurposeAccess, SessionID: "test-session-00000001", Generation: 1, RefreshID: "test-refresh-00000001", ExpiresAt: time.Now().Add(time.Hour)}}
			service := application.NewService(userRepositoryFake{user: domain.User{ID: 42, Status: test.status}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, newSessionFake(), authorizationFake{}, tokens)
			identity, err := service.Authenticate(context.Background(), "access")
			code, _ := application.CodeOf(err)
			if code != test.wantCode || identity.UserID != 0 {
				t.Fatalf("Authenticate() = %#v, %v; code=%q want %q", identity, err, code, test.wantCode)
			}
		})
	}
}

func loginSessionService(sessions *sessionFake, tokens *tokenFake) *application.Service {
	return application.NewService(userRepositoryFake{user: domain.User{ID: 42, Username: "alice", Password: "stored", Status: 1}}, &captchaFake{verified: true}, passwordFake{}, &loginAttemptFake{}, sessions, authorizationFake{}, tokens)
}

func TestRefreshRecoversCommittedMaterialWithoutPreparingAnotherCandidate(t *testing.T) {
	ctx := context.Background()
	sessions, tokens := newSessionFake(), &tokenFake{}
	service := loginSessionService(sessions, tokens)
	login, err := service.Login(ctx, application.LoginRequest{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	request := application.RefreshRequest{RefreshToken: login.RefreshToken, AttemptID: "recovery-attempt-00001"}
	first, err := service.Refresh(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	deadline := sessions.results[login.SessionID+":"+request.AttemptID].deadline
	sessionExpiry := sessions.sessions[login.SessionID].ExpiresAt
	prepared := tokens.prepareCalls
	tokens.prepareErr = errors.New("entropy unavailable")
	for _, credential := range []string{login.RefreshToken, first.RefreshToken} {
		request.RefreshToken = credential
		recovered, err := service.Refresh(ctx, request)
		if err != nil {
			t.Fatalf("committed recovery failed: %v", err)
		}
		if recovered.AccessToken != first.AccessToken || recovered.RefreshToken != first.RefreshToken {
			t.Fatal("recovery changed signed result")
		}
	}
	if tokens.prepareCalls != prepared || !sessions.results[login.SessionID+":"+request.AttemptID].deadline.Equal(deadline) || !sessions.sessions[login.SessionID].ExpiresAt.Equal(sessionExpiry) {
		t.Fatal("recovery prepared material or extended a deadline")
	}
	if _, err := service.Authenticate(ctx, login.AccessToken); err != nil {
		t.Fatalf("normal rotation rejected earlier access: %v", err)
	}
}

func TestRefreshSigningFailureAfterCommitRemainsRecoverable(t *testing.T) {
	ctx := context.Background()
	sessions, tokens := newSessionFake(), &tokenFake{}
	service := loginSessionService(sessions, tokens)
	login, err := service.Login(ctx, application.LoginRequest{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	tokens.issueErr = errors.New("signing unavailable")
	request := application.RefreshRequest{RefreshToken: login.RefreshToken, AttemptID: "signing-recovery-00001"}
	rejected, err := service.Refresh(ctx, request)
	if code, _ := application.CodeOf(err); code != application.CodeInternalError || rejected.AccessToken != "" || rejected.RefreshToken != "" {
		t.Fatal("real signing failure was not an empty internal error")
	}
	result, committed := sessions.results[login.SessionID+":"+request.AttemptID]
	if !committed || sessions.sessions[login.SessionID].Generation != 2 {
		t.Fatal("signing failure lost committed recovery material")
	}
	tokens.issueErr, tokens.prepareErr = nil, errors.New("fresh preparation must not run")
	recovered, err := service.Refresh(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := jwtadapter.NewService(jwtadapter.Config{Secret: "fixture-signing-secret", AccessExpireMins: 15, RefreshExpireMins: 60}).Issue(ctx, result.material)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.AccessToken != expected.AccessToken || recovered.RefreshToken != expected.RefreshToken || tokens.prepareCalls != 1 {
		t.Fatal("post-commit recovery did not reconstruct the committed result")
	}
}

func TestLoginFailureNeverReplacesExistingBrowserSession(t *testing.T) {
	ctx := context.Background()
	sessions, tokens := newSessionFake(), &tokenFake{}
	service := loginSessionService(sessions, tokens)
	original, err := service.Login(ctx, application.LoginRequest{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"signing", "session commit"} {
		t.Run(stage, func(t *testing.T) {
			if stage == "signing" {
				tokens.issueErr = errors.New("signing unavailable")
			} else {
				tokens.issueErr = nil
				sessions.createErr = errors.New("session store unavailable")
			}
			result, err := service.Login(ctx, application.LoginRequest{Username: "alice", PreviousRefreshToken: original.RefreshToken})
			if code, _ := application.CodeOf(err); code != application.CodeInternalError || result.AccessToken != "" || result.RefreshToken != "" {
				t.Fatal("failed login published credentials")
			}
			if _, err := service.Authenticate(ctx, original.AccessToken); err != nil {
				t.Fatal("failed login replaced the original active session")
			}
		})
	}
}

func TestLogoutRevokesOnlyCookieSessionAndReportsRealFailure(t *testing.T) {
	ctx := context.Background()
	sessions, tokens := newSessionFake(), &tokenFake{}
	service := loginSessionService(sessions, tokens)
	first, err := service.Login(ctx, application.LoginRequest{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Login(ctx, application.LoginRequest{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	sessions.revokeErr = errors.New("redis unavailable")
	if code, _ := application.CodeOf(service.Logout(ctx, first.RefreshToken)); code != application.CodeInternalError {
		t.Fatal("failed server revocation reported success")
	}
	if _, err := service.Authenticate(ctx, first.AccessToken); err != nil {
		t.Fatal("failed revocation removed active session")
	}
	sessions.revokeErr = nil
	for _, cookie := range []string{first.RefreshToken, first.RefreshToken, "", "invalid", first.AccessToken} {
		if err := service.Logout(ctx, cookie); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Authenticate(ctx, first.AccessToken); err == nil {
		t.Fatal("revoked session access remained usable")
	}
	if _, err := service.Authenticate(ctx, other.AccessToken); err != nil {
		t.Fatal("logout affected independent session")
	}
	replacement, err := service.Login(ctx, application.LoginRequest{Username: "alice", PreviousRefreshToken: other.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, other.AccessToken); err == nil {
		t.Fatal("successful browser replacement preserved old session")
	}
	if _, err := service.Authenticate(ctx, replacement.AccessToken); err != nil {
		t.Fatal("replacement session not usable")
	}
}

func TestSessionStoreFaultsAreNotAuthenticationRejections(t *testing.T) {
	ctx := context.Background()
	sessions, tokens := newSessionFake(), &tokenFake{}
	service := loginSessionService(sessions, tokens)
	login, err := service.Login(ctx, application.LoginRequest{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	sessions.readErr = errors.New("redis query failed")
	if _, err := service.Authenticate(ctx, login.AccessToken); err == nil {
		t.Fatal("session query failure accepted")
	} else if code, _ := application.CodeOf(err); code != application.CodeInternalError {
		t.Fatal("session query failure misclassified")
	}
	sessions.readErr = nil
	sessions.rotateErr = errors.New("redis rotation failed")
	if _, err := service.Refresh(ctx, application.RefreshRequest{RefreshToken: login.RefreshToken, AttemptID: "failure-attempt-00001"}); err == nil {
		t.Fatal("rotation failure accepted")
	} else if code, _ := application.CodeOf(err); code != application.CodeInternalError {
		t.Fatal("rotation failure misclassified")
	}
	sessions.rotateErr = nil
	delete(sessions.sessions, login.SessionID)
	if _, err := service.Authenticate(ctx, login.AccessToken); err == nil {
		t.Fatal("missing session accepted")
	} else if code, _ := application.CodeOf(err); code != application.CodeTokenInvalid {
		t.Fatal("session missing misclassified")
	}
	if _, err := service.Refresh(ctx, application.RefreshRequest{RefreshToken: login.RefreshToken, AttemptID: "failure-attempt-00001"}); !errors.Is(err, application.ErrRefreshTokenInvalid) {
		t.Fatal("missing session refresh did not reject")
	}
}
