package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	identitygorm "admin/internal/identity/adapters/gorm"
	jwtadapter "admin/internal/identity/adapters/jwt"
	"admin/internal/identity/adapters/password"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	"admin/testsupport/testutil"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// sessionAuthorization reads the real owner store. The optional interleaving
// injects a SQL fault only after Ensure has successfully committed, not in it.
// Authorization roles/permissions and App's cross-module bridge are out of scope.
type sessionAuthorization struct {
	versions    *authgorm.AccessVersions
	afterEnsure func()
}

func (authorization *sessionAuthorization) EnsureVersion(ctx context.Context, userID uint) (int, error) {
	version, err := authorization.versions.Ensure(ctx, userID)
	if err == nil && authorization.afterEnsure != nil {
		authorization.afterEnsure()
	}
	return version, err
}

func (authorization *sessionAuthorization) Snapshot(ctx context.Context, userID uint) (domain.AccessSnapshot, error) {
	version, err := authorization.versions.Current(ctx, userID)
	return domain.AccessSnapshot{Version: version}, err
}

func sessionFixture(t *testing.T) (*gorm.DB, *identitygorm.Repository, *sessionAuthorization, *jwtadapter.Service, *application.Service, domain.User) {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&identitygorm.UserModel{}, &authgorm.UserAccessVersion{}); err != nil {
		t.Fatal(err)
	}
	hasher := password.Bcrypt{Cost: 4}
	hash, err := hasher.Hash("ValidPass123!")
	if err != nil {
		t.Fatal(err)
	}
	repository := identitygorm.NewRepository(db)
	user := domain.User{Username: "session-user", Email: "session@example.test", Password: hash, Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatal(err)
	}
	authorization := &sessionAuthorization{versions: authgorm.NewAccessVersions(db)}
	tokens := jwtadapter.NewService(jwtadapter.Config{Secret: "session-regression-secret", AccessExpireMins: 15, RefreshExpireMins: 60})
	service := application.NewService(repository, &captchaFake{verified: true}, hasher, &loginAttemptFake{}, blacklistFake{}, authorization, tokens)
	return db, repository, authorization, tokens, service, user
}

func sessionLogin(service *application.Service) (application.LoginResult, error) {
	return service.Login(context.Background(), application.LoginRequest{Username: "session-user", Password: "ValidPass123!", CaptchaID: "captcha", CaptchaCode: "123456"})
}

func assertSessionPair(t *testing.T, tokens *jwtadapter.Service, pair application.TokenPair, userID uint, version int) {
	t.Helper()
	for _, test := range []struct {
		token   string
		purpose application.TokenPurpose
	}{
		{pair.AccessToken, application.TokenPurposeAccess},
		{pair.RefreshToken, application.TokenPurposeRefresh},
	} {
		claims, err := tokens.Parse(context.Background(), test.token)
		if err != nil || claims.UserID != userID || claims.TokenVersion != version || claims.Purpose != test.purpose {
			t.Fatalf("issued %s claims = %#v, %v; want user %d version %d", test.purpose, claims, err, userID, version)
		}
	}
}

func TestLoginInitializesAndUsesPersistedAuthorizationVersion(t *testing.T) {
	db, repository, authorization, tokens, service, user := sessionFixture(t)
	ctx := context.Background()
	if _, err := authorization.versions.Current(ctx, user.ID); !errors.Is(err, authgorm.ErrAccessVersionNotFound) {
		t.Fatalf("new user version error = %v", err)
	}
	for _, version := range []int{1, 7} {
		if version == 7 {
			if err := db.Model(&authgorm.UserAccessVersion{}).Where("user_id = ?", user.ID).Update("version", version).Error; err != nil {
				t.Fatal(err)
			}
		}
		result, err := sessionLogin(service)
		if err != nil {
			t.Fatalf("Login() at version %d: %v", version, err)
		}
		assertSessionPair(t, tokens, application.TokenPair{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken}, user.ID, version)
		if current, err := authorization.versions.Current(ctx, user.ID); err != nil || current != version {
			t.Fatalf("persisted version = %d, %v; want %d", current, err, version)
		}
	}
	if after, err := repository.FindByID(ctx, user.ID); err != nil || !reflect.DeepEqual(after, user) {
		t.Fatalf("login changed persisted identity: %#v, %v", after, err)
	}
}

func TestLoginVersionStorageFailuresReturnSafeInternalWithoutTokens(t *testing.T) {
	for _, phase := range []string{"ensure create", "ensure read", "snapshot after ensure"} {
		t.Run(phase, func(t *testing.T) {
			db, repository, authorization, tokens, service, user := sessionFixture(t)
			ctx := context.Background()
			restore := func() {}
			switch phase {
			case "ensure create":
				if err := db.Exec("CREATE TRIGGER reject_session_version BEFORE INSERT ON user_access_versions BEGIN SELECT RAISE(ABORT, 'private version insert fault'); END").Error; err != nil {
					t.Fatal(err)
				}
				restore = func() {
					if err := db.Exec("DROP TRIGGER reject_session_version").Error; err != nil {
						t.Fatal(err)
					}
				}
			case "ensure read", "snapshot after ensure":
				removeVersionTable := func() {
					if err := db.Exec("ALTER TABLE user_access_versions RENAME TO held_session_versions").Error; err != nil {
						t.Fatal(err)
					}
				}
				if phase == "ensure read" {
					removeVersionTable()
				} else {
					authorization.afterEnsure = removeVersionTable
				}
				restore = func() {
					if err := db.Exec("ALTER TABLE held_session_versions RENAME TO user_access_versions").Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := sessionLogin(service)
			var classified *application.Error
			if !errors.As(err, &classified) || classified.Code != application.CodeInternalError || errors.Unwrap(classified) == nil {
				t.Fatalf("Login() error = %v; want typed internal error retaining storage cause", err)
			}
			if err.Error() != application.NewError(application.CodeInternalError, nil).Error() {
				t.Fatalf("Login() exposed storage failure: %v", err)
			}
			if result.AccessToken != "" || result.RefreshToken != "" {
				t.Fatal("failed login returned a token pair")
			}
			restore()
			authorization.afterEnsure = nil
			current, currentErr := authorization.versions.Current(ctx, user.ID)
			if phase == "snapshot after ensure" {
				if currentErr != nil || current != 1 {
					t.Fatalf("successful Ensure must persist version 1: %d, %v", current, currentErr)
				}
			} else if !errors.Is(currentErr, authgorm.ErrAccessVersionNotFound) {
				t.Fatalf("failed Ensure left version %d, %v; want absent", current, currentErr)
			}
			if after, err := repository.FindByID(ctx, user.ID); err != nil || !reflect.DeepEqual(after, user) {
				t.Fatalf("failed login changed persisted credentials/identity: %#v, %v", after, err)
			}
			// Removing only the version-store fault restores the same valid login.
			result, err = sessionLogin(service)
			if err != nil {
				t.Fatalf("Login() after fault removal failed: %v", err)
			}
			assertSessionPair(t, tokens, application.TokenPair{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken}, user.ID, 1)
		})
	}
}

func TestRefreshRejectsMalformedExpiredAndAccessTokensWithoutChangingVersions(t *testing.T) {
	for _, condition := range []string{"malformed", "expired", "access purpose"} {
		for _, versionExists := range []bool{true, false} {
			name := condition + "/version absent"
			if versionExists {
				name = condition + "/version present"
			}
			t.Run(name, func(t *testing.T) {
				db, repository, authorization, tokens, service, user := sessionFixture(t)
				ctx := context.Background()
				if err := db.Create(&authgorm.UserAccessVersion{UserID: user.ID, Version: 7}).Error; err != nil {
					t.Fatal(err)
				}
				valid, err := tokens.Issue(ctx, application.TokenIssue{UserID: user.ID, TokenVersion: 7})
				if err != nil {
					t.Fatal(err)
				}
				// The same user, blacklist and current version admit a real refresh.
				control, err := service.Refresh(ctx, application.RefreshRequest{RefreshToken: valid.RefreshToken})
				if err != nil {
					t.Fatalf("valid refresh control: %v", err)
				}
				assertSessionPair(t, tokens, control, user.ID, 7)
				token := "not.a.valid.jwt"
				switch condition {
				case "expired":
					// Fixed historical dates make expiration deterministic without sleeps.
					token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
						"user_id": user.ID, "token_version": 7, "token_type": "refresh",
						"iat": 946684800, "exp": 946688400,
					}).SignedString([]byte("session-regression-secret"))
					if err != nil {
						t.Fatal(err)
					}
					if _, err := jwt.Parse(token, func(*jwt.Token) (any, error) {
						return []byte("session-regression-secret"), nil
					}, jwt.WithoutClaimsValidation()); err != nil {
						t.Fatalf("expired fixture signature is invalid: %v", err)
					}
					if _, err := tokens.Parse(ctx, token); !errors.Is(err, jwt.ErrTokenExpired) {
						t.Fatalf("expired fixture rejection = %v; want expiration", err)
					}
				case "access purpose":
					token = valid.AccessToken
					claims, err := tokens.Parse(ctx, token)
					if err != nil || claims.Purpose != application.TokenPurposeAccess || claims.UserID != user.ID || claims.TokenVersion != 7 {
						t.Fatalf("access fixture is not otherwise valid: %#v, %v", claims, err)
					}
				}
				if !versionExists {
					if err := db.Delete(&authgorm.UserAccessVersion{}, "user_id = ?", user.ID).Error; err != nil {
						t.Fatal(err)
					}
				}
				pair, err := service.Refresh(ctx, application.RefreshRequest{RefreshToken: token})
				assertGenericRefreshRejection(t, pair, err)
				current, currentErr := authorization.versions.Current(ctx, user.ID)
				if versionExists {
					if currentErr != nil || current != 7 {
						t.Fatalf("refresh changed current version: %d, %v", current, currentErr)
					}
				} else if !errors.Is(currentErr, authgorm.ErrAccessVersionNotFound) {
					t.Fatalf("refresh repaired missing version: %d, %v", current, currentErr)
				}
				if after, err := repository.FindByID(ctx, user.ID); err != nil || !reflect.DeepEqual(after, user) {
					t.Fatalf("refresh changed persisted identity: %#v, %v", after, err)
				}
			})
		}
	}
}

func assertGenericRefreshRejection(t *testing.T, pair application.TokenPair, err error) {
	t.Helper()
	code, ok := application.CodeOf(err)
	if !ok || code != application.CodeRefreshTokenInvalid || !errors.Is(err, application.ErrRefreshTokenInvalid) || err.Error() != application.ErrRefreshTokenInvalid.Error() || errors.Unwrap(err) != nil {
		t.Fatalf("Refresh() error = %v; want the same cause-free generic refresh rejection", err)
	}
	if pair.AccessToken != "" || pair.RefreshToken != "" {
		t.Fatal("rejected refresh returned a token pair")
	}
}

func TestRefreshRejectsValidTokenForAbsentUserWithoutInitializingVersion(t *testing.T) {
	for _, versionExists := range []bool{true, false} {
		name := "version absent"
		if versionExists {
			name = "version present"
		}
		t.Run(name, func(t *testing.T) {
			db, repository, authorization, tokens, service, user := sessionFixture(t)
			ctx := context.Background()
			if err := db.Create(&authgorm.UserAccessVersion{UserID: user.ID, Version: 7}).Error; err != nil {
				t.Fatal(err)
			}
			pair, err := tokens.Issue(ctx, application.TokenIssue{UserID: user.ID, TokenVersion: 7})
			if err != nil {
				t.Fatal(err)
			}
			claims, err := tokens.Parse(ctx, pair.RefreshToken)
			if err != nil || claims.UserID != user.ID || claims.TokenVersion != 7 || claims.Purpose != application.TokenPurposeRefresh {
				t.Fatalf("missing-user refresh fixture is not otherwise valid: %#v, %v", claims, err)
			}
			control, err := service.Refresh(ctx, application.RefreshRequest{RefreshToken: pair.RefreshToken})
			if err != nil {
				t.Fatalf("refresh before removing user: %v", err)
			}
			assertSessionPair(t, tokens, control, user.ID, 7)
			if err := db.Delete(&identitygorm.UserModel{}, user.ID).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := repository.FindByID(ctx, user.ID); !errors.Is(err, application.ErrUserNotFound) {
				t.Fatalf("missing-user premise: %v", err)
			}
			if !versionExists {
				if err := db.Delete(&authgorm.UserAccessVersion{}, "user_id = ?", user.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			var before identitygorm.UserModel
			if err := db.Unscoped().First(&before, user.ID).Error; err != nil {
				t.Fatal(err)
			}
			result, err := service.Refresh(ctx, application.RefreshRequest{RefreshToken: pair.RefreshToken})
			assertGenericRefreshRejection(t, result, err)
			version, versionErr := authorization.versions.Current(ctx, user.ID)
			if versionExists {
				if versionErr != nil || version != 7 {
					t.Fatalf("refresh changed absent user's version: %d, %v", version, versionErr)
				}
			} else if !errors.Is(versionErr, authgorm.ErrAccessVersionNotFound) {
				t.Fatalf("refresh initialized absent user's version: %d, %v", version, versionErr)
			}
			if _, err := repository.FindByID(ctx, user.ID); !errors.Is(err, application.ErrUserNotFound) {
				t.Fatalf("refresh restored missing identity: %v", err)
			}
			var after identitygorm.UserModel
			if err := db.Unscoped().First(&after, user.ID).Error; err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("refresh changed deleted user's persisted credentials/identity: %#v, %v", after, err)
			}
		})
	}
}
