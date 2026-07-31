package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"admin/dto"
	"admin/global"
	"admin/model"
	"admin/utils"
)

const tokenVersionRegressionSecret = "token-version-regression-secret"

func TestLoginInitializesMissingAccessVersionAfterCredentialsSucceed(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)

	const (
		captchaID   = "initialize-access-version-captcha"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "initialize-access-version-user",
		Password:     string(passwordHash),
		Email:        "initialize-access-version-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create login user without access version: %v", err)
	}

	response, err := Login(dto.LoginReq{
		Username:    user.Username,
		Password:    "ValidPass123!",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if response == nil || response.AccessToken == "" || response.RefreshToken == "" {
		t.Fatalf("Login() response = %#v, want access and refresh tokens", response)
	}

	var accessVersion model.UserAccessVersion
	if err := db.First(&accessVersion, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("read lazily initialized access version: %v", err)
	}
	if accessVersion.Version != 1 {
		t.Fatalf("initialized access version = %d, want 1", accessVersion.Version)
	}
}

func TestLoginRecordsMissingAuthorizationVersionAfterTokenIssuance(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)

	metrics := NewAccessVersionMetrics()
	previousMetrics := defaultAccessVersionMetrics
	defaultAccessVersionMetrics = metrics
	t.Cleanup(func() {
		defaultAccessVersionMetrics = previousMetrics
	})

	const (
		captchaID   = "missing-after-token-issuance-captcha"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "missing-after-token-issuance-user",
		Password:     string(passwordHash),
		Email:        "missing-after-token-issuance-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create login user without access version: %v", err)
	}
	accessVersionReads := 0
	if err := db.Callback().Query().
		After("gorm:query").
		Register(
			"test:remove-access-version-after-login-initialization",
			func(tx *gorm.DB) {
				if tx.Statement.Schema == nil ||
					tx.Statement.Schema.Name != "UserAccessVersion" {
					return
				}
				accessVersionReads++
				if accessVersionReads != 2 {
					return
				}
				if err := tx.Exec(
					"DELETE FROM user_access_versions WHERE user_id = ?",
					user.ID,
				).Error; err != nil {
					tx.AddError(err)
				}
			},
		); err != nil {
		t.Fatalf("register post-initialization missing-row callback: %v", err)
	}

	response, err := Login(dto.LoginReq{
		Username:    user.Username,
		Password:    "ValidPass123!",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if response == nil || response.AccessToken == "" || response.RefreshToken == "" {
		t.Fatalf("Login() response = %#v, want successfully issued token pair", response)
	}

	snapshot := metrics.Snapshot()
	if snapshot.TokenIssuanceMissing != 1 {
		t.Fatalf(
			"token-issuance-missing metric = %d, want 1",
			snapshot.TokenIssuanceMissing,
		)
	}
	if snapshot.UnexpectedMissing != 0 {
		t.Fatalf(
			"generic unexpected-missing metric = %d, want 0",
			snapshot.UnexpectedMissing,
		)
	}
}

func TestLoginReturnsNoTokensWhenAccessVersionInitializationFails(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)

	const (
		captchaID   = "failed-access-version-captcha"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "failed-access-version-user",
		Password:     string(passwordHash),
		Email:        "failed-access-version-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create login user without access version: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_login_access_version_initialization
		BEFORE INSERT ON user_access_versions
		BEGIN
			SELECT RAISE(ABORT, 'forced access-version initialization failure');
		END
	`).Error; err != nil {
		t.Fatalf("create access-version initialization failure trigger: %v", err)
	}

	response, err := Login(dto.LoginReq{
		Username:    user.Username,
		Password:    "ValidPass123!",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if err == nil {
		t.Fatal("Login() error = nil, want access-version initialization failure")
	}
	if response != nil {
		t.Fatalf("Login() response = %#v, want nil so no token can be issued", response)
	}

	var count int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", user.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count access versions after failed login: %v", err)
	}
	if count != 0 {
		t.Fatalf("access-version rows after failed login = %d, want 0", count)
	}
}

func TestLoginRollsBackInitializationAndReturnsNoTokensWhenVersionRereadFails(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)

	const (
		captchaID   = "failed-access-version-reread-captcha"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "failed-access-version-reread-user",
		Password:     string(passwordHash),
		Email:        "failed-access-version-reread-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 4,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create login user without access version: %v", err)
	}

	forcedRereadFailure := errors.New("forced access-version reread failure")
	accessVersionReads := 0
	if err := db.Callback().Query().
		Before("gorm:query").
		Register("test:fail-login-access-version-reread", func(tx *gorm.DB) {
			if tx.Statement.Schema == nil ||
				tx.Statement.Schema.Name != "UserAccessVersion" {
				return
			}
			accessVersionReads++
			if accessVersionReads == 2 {
				tx.AddError(forcedRereadFailure)
			}
		}); err != nil {
		t.Fatalf("register access-version reread failure: %v", err)
	}

	response, err := Login(dto.LoginReq{
		Username:    user.Username,
		Password:    "ValidPass123!",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if !errors.Is(err, forcedRereadFailure) {
		t.Fatalf("Login() error = %v, want forced reread failure", err)
	}
	if response != nil {
		t.Fatalf("Login() response = %#v, want nil so no token can be issued", response)
	}

	var count int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", user.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count access versions after failed reread: %v", err)
	}
	if count != 0 {
		t.Fatalf("access-version rows after failed reread = %d, want rolled back 0", count)
	}

}

func TestLoginAccessTokenUsesAuthorizationAccessVersion(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)

	const (
		captchaID   = "access-token-version-captcha"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "access-token-version-user",
		Password:     string(passwordHash),
		Email:        "access-token-version-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 9,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create login user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: 3,
	}).Error; err != nil {
		t.Fatalf("create authorization access version: %v", err)
	}

	response, err := Login(dto.LoginReq{
		Username:    user.Username,
		Password:    "ValidPass123!",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	accessClaims := parseTokenVersionRegressionToken(t, response.AccessToken)
	if accessClaims.TokenVersion != 3 {
		t.Fatalf(
			"access token version = %d, want authorization-owned version 3 instead of legacy version %d",
			accessClaims.TokenVersion,
			user.TokenVersion,
		)
	}
}

func TestLoginRefreshTokenUsesAndValidatesAuthorizationAccessVersion(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)

	const (
		captchaID   = "refresh-token-version-captcha"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "refresh-token-version-user",
		Password:     string(passwordHash),
		Email:        "refresh-token-version-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 9,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create login user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: 3,
	}).Error; err != nil {
		t.Fatalf("create authorization access version: %v", err)
	}

	response, err := Login(dto.LoginReq{
		Username:    user.Username,
		Password:    "ValidPass123!",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	refreshClaims := parseTokenVersionRegressionToken(t, response.RefreshToken)
	if refreshClaims.TokenVersion != 3 {
		t.Fatalf(
			"refresh token version = %d, want authorization-owned version 3 instead of legacy version %d",
			refreshClaims.TokenVersion,
			user.TokenVersion,
		)
	}
	if err := IsTokenVersionValid(
		refreshClaims.UserID,
		refreshClaims.TokenVersion,
	); err != nil {
		t.Fatalf(
			"refresh token matching authorization version was rejected after only legacy field diverged: %v",
			err,
		)
	}
}

func TestRefreshTokensRecordsMissingAuthorizationVersionAfterTokenIssuance(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)
	redisClient.AddHook(tokenVersionRegressionRedisHook{})

	metrics := NewAccessVersionMetrics()
	previousMetrics := defaultAccessVersionMetrics
	defaultAccessVersionMetrics = metrics
	t.Cleanup(func() {
		defaultAccessVersionMetrics = previousMetrics
	})

	user := exitAccessVersionTestUser{
		Username:     "refresh-missing-after-token-issuance-user",
		Password:     "not-used",
		Email:        "refresh-missing-after-token-issuance-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 3,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create refresh user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create refresh user access version: %v", err)
	}
	_, refreshToken := generateTokenVersionRegressionPair(t, user)

	removedAccessVersion := false
	if err := db.Callback().Query().
		After("gorm:query").
		Register(
			"test:remove-access-version-after-refresh-read",
			func(tx *gorm.DB) {
				if removedAccessVersion ||
					tx.Statement.Schema == nil ||
					tx.Statement.Schema.Name != "UserAccessVersion" {
					return
				}
				removedAccessVersion = true
				if err := db.Exec(
					"DELETE FROM user_access_versions WHERE user_id = ?",
					user.ID,
				).Error; err != nil {
					tx.AddError(err)
				}
			},
		); err != nil {
		t.Fatalf("register refresh missing-row callback: %v", err)
	}

	response, err := RefreshTokens(dto.RefreshTokenReq{
		RefreshToken: refreshToken,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if err != nil {
		t.Fatalf("RefreshTokens() error = %v", err)
	}
	if response == nil || response.AccessToken == "" || response.RefreshToken == "" {
		t.Fatalf(
			"RefreshTokens() response = %#v, want successfully issued token pair",
			response,
		)
	}

	snapshot := metrics.Snapshot()
	if snapshot.TokenIssuanceMissing != 1 {
		t.Fatalf(
			"token-issuance-missing metric = %d, want 1",
			snapshot.TokenIssuanceMissing,
		)
	}
	if snapshot.UnexpectedMissing != 0 {
		t.Fatalf(
			"generic unexpected-missing metric = %d, want 0",
			snapshot.UnexpectedMissing,
		)
	}
}

func TestLoginIssuesAccessAndRefreshTokensWithCurrentVersion(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)

	const (
		captchaID   = "login-captcha"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("ValidPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "login-user",
		Password:     string(passwordHash),
		Email:        "login-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 7,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create login user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create migrated access version: %v", err)
	}

	response, err := Login(dto.LoginReq{
		Username:    user.Username,
		Password:    "ValidPass123!",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	}, JWTConfig{
		Secret:            tokenVersionRegressionSecret,
		ExpireMins:        15,
		RefreshExpireMins: 60,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if response == nil || response.AccessToken == "" || response.RefreshToken == "" {
		t.Fatalf("Login() response = %#v, want access and refresh tokens", response)
	}

	accessClaims := parseTokenVersionRegressionToken(t, response.AccessToken)
	refreshClaims := parseTokenVersionRegressionToken(t, response.RefreshToken)
	for name, claims := range map[string]*utils.Claims{
		"access":  accessClaims,
		"refresh": refreshClaims,
	} {
		if claims.UserID != user.ID {
			t.Fatalf("%s token user ID = %d, want %d", name, claims.UserID, user.ID)
		}
		if claims.TokenVersion != user.TokenVersion {
			t.Fatalf(
				"%s token version = %d, want current version %d",
				name,
				claims.TokenVersion,
				user.TokenVersion,
			)
		}
		if err := IsTokenVersionValid(claims.UserID, claims.TokenVersion); err != nil {
			t.Fatalf("%s token rejected before invalidation: %v", name, err)
		}
	}
	if !refreshClaims.ExpiresAt.After(accessClaims.ExpiresAt.Time) {
		t.Fatal("refresh token must expire after access token")
	}
}

func TestDisablingUserInvalidatesExistingAccessAndRefreshTokens(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: target.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}
	accessToken, refreshToken := generateTokenVersionRegressionPair(t, target)

	status, err := ToggleUserStatus(operator.ID, target.ID)
	if err != nil {
		t.Fatalf("ToggleUserStatus() error = %v", err)
	}
	if status != 0 {
		t.Fatalf("ToggleUserStatus() status = %d, want disabled status 0", status)
	}

	assertTokenVersionRegressionState(t, db, target.ID, 0, target.TokenVersion)
	assertAuthorizationVersion(t, db, target.ID, target.TokenVersion+1)
	assertTokenVersionRegressionPairRejected(t, accessToken, refreshToken)
}

func TestKickInvalidatesExistingAccessAndRefreshTokensWithoutChangingStatus(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: target.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}
	accessToken, refreshToken := generateTokenVersionRegressionPair(t, target)

	if err := KickUserByAdmin(operator.ID, target.ID); err != nil {
		t.Fatalf("KickUserByAdmin() error = %v", err)
	}

	assertTokenVersionRegressionState(t, db, target.ID, 1, target.TokenVersion)
	assertAuthorizationVersion(t, db, target.ID, target.TokenVersion+1)
	assertTokenVersionRegressionPairRejected(t, accessToken, refreshToken)
}

func TestChangingPasswordUsesAuthorizationVersionAndInvalidatesExistingTokens(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte("OldPass123!"),
		bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "change-password-user",
		Password:     string(passwordHash),
		Email:        "change-password-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 3,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create password user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create password user access version: %v", err)
	}
	accessToken, refreshToken := generateTokenVersionRegressionPair(t, user)

	err = ChangePassword(user.ID, dto.ChangePasswordReq{
		OldPassword:     "OldPass123!",
		NewPassword:     "NewPass456!",
		ConfirmPassword: "NewPass456!",
	})
	if err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	var stored exitAccessVersionTestUser
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatalf("reload password user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword(
		[]byte(stored.Password),
		[]byte("NewPass456!"),
	); err != nil {
		t.Fatalf("stored password does not match new password: %v", err)
	}
	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
	assertTokenVersionRegressionPairRejected(t, accessToken, refreshToken)
}

func TestAdminRoleOrStatusUpdateUsesAuthorizationVersion(t *testing.T) {
	testCases := []struct {
		name       string
		request    dto.AdminUpdateUserReq
		wantRole   string
		wantStatus int
	}{
		{
			name:       "role change",
			request:    dto.AdminUpdateUserReq{Role: "manager"},
			wantRole:   "manager",
			wantStatus: 1,
		},
		{
			name: "status change",
			request: dto.AdminUpdateUserReq{
				Status: intPointer(0),
			},
			wantRole:   "user",
			wantStatus: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTokenVersionRegressionDB(t)
			installTokenVersionRegressionGlobals(t, db)
			operator, target := createTokenVersionRegressionUsers(t, db)
			if err := db.Create(&model.UserAccessVersion{
				UserID:  target.ID,
				Version: target.TokenVersion,
			}).Error; err != nil {
				t.Fatalf("create target access version: %v", err)
			}
			accessToken, refreshToken :=
				generateTokenVersionRegressionPair(t, target)

			response, err := UpdateUserByAdmin(
				operator.ID,
				target.ID,
				tc.request,
			)
			if err != nil {
				t.Fatalf("UpdateUserByAdmin() error = %v", err)
			}
			if response.Role != tc.wantRole || response.Status != tc.wantStatus {
				t.Fatalf(
					"updated user role/status = %s/%d, want %s/%d",
					response.Role,
					response.Status,
					tc.wantRole,
					tc.wantStatus,
				)
			}

			assertTokenVersionRegressionState(
				t,
				db,
				target.ID,
				tc.wantStatus,
				target.TokenVersion,
			)
			assertAuthorizationVersion(
				t,
				db,
				target.ID,
				target.TokenVersion+1,
			)
			assertTokenVersionRegressionPairRejected(
				t,
				accessToken,
				refreshToken,
			)
		})
	}
}

func TestPermissionChangeInvalidatesExistingAccessAndRefreshTokens(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	role := model.Role{
		Name:      "operator",
		Code:      "operator",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "permission-user",
		Password:     "not-used",
		Email:        "permission-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 3,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create permission user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: user.ID,
		RoleID: role.ID,
	}).Error; err != nil {
		t.Fatalf("assign role to user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create permission user access version: %v", err)
	}
	permission := model.Permission{
		Name: "读取用户",
		Code: "user:read",
	}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	accessToken, refreshToken := generateTokenVersionRegressionPair(t, user)

	if err := AssignPermissionsToRole(role.ID, []uint{permission.ID}); err != nil {
		t.Fatalf("AssignPermissionsToRole() error = %v", err)
	}

	assertTokenVersionRegressionState(t, db, user.ID, 1, user.TokenVersion)
	assertAuthorizationVersion(t, db, user.ID, user.TokenVersion+1)
	assertTokenVersionRegressionPairRejected(t, accessToken, refreshToken)
}

func TestTokensIssuedBeforeMigrationRemainValidAfterAuthorizationVersionBackfill(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)
	redisClient.AddHook(tokenVersionRegressionRedisHook{})

	user := exitAccessVersionTestUser{
		Username:     "pre-migration-token-user",
		Password:     "not-used",
		Email:        "pre-migration-token-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 7,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create pre-migration token user: %v", err)
	}
	accessToken, refreshToken := generateLegacyTokenVersionRegressionPair(t, user)

	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("backfill authorization access version: %v", err)
	}
	if _, err := RefreshTokens(dto.RefreshTokenReq{
		RefreshToken: accessToken,
	}, JWTConfig{
		Secret:                  tokenVersionRegressionSecret,
		ExpireMins:              15,
		RefreshExpireMins:       60,
		LegacyAccessExpireMins:  15,
		LegacyRefreshExpireMins: 60,
	}); !errors.Is(err, ErrRefreshTokenInvalid) {
		t.Fatalf(
			"pre-migration access token refresh error = %v, want %v",
			err,
			ErrRefreshTokenInvalid,
		)
	}

	accessClaims := parseTokenVersionRegressionToken(t, accessToken)
	if err := IsTokenVersionValid(
		accessClaims.UserID,
		accessClaims.TokenVersion,
	); err != nil {
		t.Fatalf("pre-migration access token rejected after backfill: %v", err)
	}

	refreshed, err := RefreshTokens(dto.RefreshTokenReq{
		RefreshToken: refreshToken,
	}, JWTConfig{
		Secret:                  tokenVersionRegressionSecret,
		ExpireMins:              15,
		RefreshExpireMins:       60,
		LegacyAccessExpireMins:  15,
		LegacyRefreshExpireMins: 60,
	})
	if err != nil {
		t.Fatalf("pre-migration refresh token rejected after backfill: %v", err)
	}
	for name, token := range map[string]string{
		"access":  refreshed.AccessToken,
		"refresh": refreshed.RefreshToken,
	} {
		claims := parseTokenVersionRegressionToken(t, token)
		if claims.UserID != user.ID || claims.TokenVersion != user.TokenVersion {
			t.Fatalf(
				"refreshed %s token user/version = %d/%d, want %d/%d",
				name,
				claims.UserID,
				claims.TokenVersion,
				user.ID,
				user.TokenVersion,
			)
		}
	}
}

func TestLegacyTokenPurposeDoesNotFollowChangedCurrentDurations(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)
	redisClient.AddHook(tokenVersionRegressionRedisHook{})

	user := exitAccessVersionTestUser{
		Username:     "legacy-token-duration-user",
		Password:     "not-used",
		Email:        "legacy-token-duration-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 3,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create legacy-duration user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create legacy-duration access version: %v", err)
	}
	accessToken, refreshToken := generateLegacyTokenVersionRegressionPair(t, user)
	changedConfig := JWTConfig{
		Secret:                  tokenVersionRegressionSecret,
		ExpireMins:              5,
		RefreshExpireMins:       15,
		LegacyAccessExpireMins:  15,
		LegacyRefreshExpireMins: 60,
	}

	t.Run("legacy access token remains access", func(t *testing.T) {
		if _, err := RefreshTokens(dto.RefreshTokenReq{
			RefreshToken: accessToken,
		}, changedConfig); !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("legacy access token refresh error = %v, want %v", err, ErrRefreshTokenInvalid)
		}
	})
	t.Run("legacy refresh token remains refresh", func(t *testing.T) {
		if _, err := RefreshTokens(dto.RefreshTokenReq{
			RefreshToken: refreshToken,
		}, changedConfig); err != nil {
			t.Fatalf("legacy refresh token rejected after current duration change: %v", err)
		}
	})
	t.Run("legacy refresh token cannot authenticate as access", func(t *testing.T) {
		claims := parseTokenVersionRegressionToken(t, refreshToken)
		if claims.HasPurpose(
			utils.TokenPurposeAccess,
			changedConfig.LegacyTokenPurposeConfig(),
		) {
			t.Fatal("legacy refresh token was accepted as an access token")
		}
	})
	t.Run("ambiguous legacy lifetimes fail closed", func(t *testing.T) {
		claims := parseTokenVersionRegressionToken(t, accessToken)
		ambiguous := utils.LegacyTokenPurposeConfig{
			AccessExpireMins:  15,
			RefreshExpireMins: 15,
		}
		if claims.HasPurpose(utils.TokenPurposeAccess, ambiguous) ||
			claims.HasPurpose(utils.TokenPurposeRefresh, ambiguous) {
			t.Fatal("ambiguous legacy lifetime was assigned a token purpose")
		}
	})
}

func openTokenVersionRegressionDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "token-version-regression.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open token version regression database: %v", err)
	}
	if err := db.AutoMigrate(
		&exitAccessVersionTestUser{},
		&model.UserAccessVersion{},
		&model.Role{},
		&model.UserRole{},
		&model.RoleDataScope{},
		&model.Permission{},
		&model.RolePermission{},
		&model.Menu{},
		&model.RoleMenu{},
		&model.API{},
		&model.MenuAPI{},
		&model.Organization{},
		&model.UserOrganization{},
	); err != nil {
		t.Fatalf("migrate token version regression database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get token version regression sql database: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return db
}

func installTokenVersionRegressionGlobals(
	t *testing.T,
	db *gorm.DB,
) *redis.Client {
	t.Helper()

	previousDB := global.DB
	previousRedis := global.Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: "token-version-regression.invalid:6379",
	})
	global.DB = db
	global.Redis = redisClient
	t.Cleanup(func() {
		global.DB = previousDB
		global.Redis = previousRedis
		_ = redisClient.Close()
	})
	return redisClient
}

func createTokenVersionRegressionUsers(
	t *testing.T,
	db *gorm.DB,
) (exitAccessVersionTestUser, exitAccessVersionTestUser) {
	t.Helper()

	adminRole := model.Role{
		Name:      "administrator",
		Code:      "admin",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&adminRole).Error; err != nil {
		t.Fatalf("create admin role: %v", err)
	}
	operator := exitAccessVersionTestUser{
		Username:     "admin-operator",
		Password:     "not-used",
		Email:        "admin-operator@example.com",
		Role:         "admin",
		Status:       1,
		TokenVersion: 1,
	}
	target := exitAccessVersionTestUser{
		Username:     "target-user",
		Password:     "not-used",
		Email:        "target-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 3,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: operator.ID,
		RoleID: adminRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	return operator, target
}

func generateTokenVersionRegressionPair(
	t *testing.T,
	user exitAccessVersionTestUser,
) (string, string) {
	t.Helper()

	accessToken, refreshToken, err := utils.GenerateToken(
		user.ID,
		user.TokenVersion,
		nil,
		nil,
		tokenVersionRegressionSecret,
		15,
		60,
	)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	return accessToken, refreshToken
}

func generateLegacyTokenVersionRegressionPair(
	t *testing.T,
	user exitAccessVersionTestUser,
) (string, string) {
	t.Helper()

	now := time.Now()
	sign := func(expireMins int) string {
		claims := utils.Claims{
			UserID:       user.ID,
			TokenVersion: user.TokenVersion,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(
					now.Add(time.Duration(expireMins) * time.Minute),
				),
				IssuedAt: jwt.NewNumericDate(now),
			},
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
			SignedString([]byte(tokenVersionRegressionSecret))
		if err != nil {
			t.Fatalf("generate legacy token: %v", err)
		}
		return token
	}

	return sign(15), sign(60)
}

func parseTokenVersionRegressionToken(t *testing.T, token string) *utils.Claims {
	t.Helper()

	claims, err := utils.ParseToken(token, tokenVersionRegressionSecret)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	return claims
}

func intPointer(value int) *int {
	return &value
}

func assertTokenVersionRegressionPairRejected(
	t *testing.T,
	accessToken string,
	refreshToken string,
) {
	t.Helper()

	for name, token := range map[string]string{
		"access":  accessToken,
		"refresh": refreshToken,
	} {
		claims := parseTokenVersionRegressionToken(t, token)
		if err := IsTokenVersionValid(claims.UserID, claims.TokenVersion); err == nil {
			t.Fatalf("%s token with old version was still accepted", name)
		}
	}
}

func assertTokenVersionRegressionState(
	t *testing.T,
	db *gorm.DB,
	userID uint,
	wantStatus int,
	wantVersion int,
) {
	t.Helper()

	var stored exitAccessVersionTestUser
	if err := db.First(&stored, userID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if stored.Status != wantStatus {
		t.Fatalf(
			"stored user status = %d, want %d; expected authorization version %d is checked separately",
			stored.Status,
			wantStatus,
			wantVersion,
		)
	}
}

func assertAuthorizationVersion(
	t *testing.T,
	db *gorm.DB,
	userID uint,
	wantVersion int,
) {
	t.Helper()

	var accessVersion model.UserAccessVersion
	if err := db.First(
		&accessVersion,
		"user_id = ?",
		userID,
	).Error; err != nil {
		t.Fatalf("reload authorization version: %v", err)
	}
	if accessVersion.Version != wantVersion {
		t.Fatalf(
			"authorization version = %d, want %d",
			accessVersion.Version,
			wantVersion,
		)
	}
}

type tokenVersionRegressionRedisHook struct {
	captchaID   string
	captchaCode string
}

func (tokenVersionRegressionRedisHook) DialHook(
	next redis.DialHook,
) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h tokenVersionRegressionRedisHook) ProcessHook(
	next redis.ProcessHook,
) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		switch strings.ToLower(cmd.Name()) {
		case "get":
			getCmd, ok := cmd.(*redis.StringCmd)
			if !ok {
				return fmt.Errorf("unexpected GET command type %T", cmd)
			}
			key := fmt.Sprint(cmd.Args()[1])
			switch key {
			case "captcha:" + h.captchaID:
				getCmd.SetVal(h.captchaCode)
				return nil
			default:
				getCmd.SetErr(redis.Nil)
				return redis.Nil
			}
		case "del":
			delCmd, ok := cmd.(*redis.IntCmd)
			if !ok {
				return fmt.Errorf("unexpected DEL command type %T", cmd)
			}
			delCmd.SetVal(1)
			return nil
		case "exists":
			existsCmd, ok := cmd.(*redis.IntCmd)
			if !ok {
				return fmt.Errorf("unexpected EXISTS command type %T", cmd)
			}
			existsCmd.SetVal(0)
			return nil
		default:
			return fmt.Errorf("unexpected Redis command %q", cmd.Name())
		}
	}
}

func (tokenVersionRegressionRedisHook) ProcessPipelineHook(
	next redis.ProcessPipelineHook,
) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		return next(ctx, cmds)
	}
}

func TestTokenVersionRegressionTokenLifetimeSanity(t *testing.T) {
	accessToken, refreshToken, err := utils.GenerateToken(
		1,
		1,
		nil,
		nil,
		tokenVersionRegressionSecret,
		15,
		60,
	)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	accessClaims := parseTokenVersionRegressionToken(t, accessToken)
	refreshClaims := parseTokenVersionRegressionToken(t, refreshToken)
	if refreshClaims.ExpiresAt.Time.Sub(accessClaims.ExpiresAt.Time) < 44*time.Minute {
		t.Fatal("refresh token lifetime must remain materially longer than access token")
	}
}
