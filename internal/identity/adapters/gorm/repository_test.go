package gormadapter_test

import (
	identitymodule "admin/internal/identity"
	identitygorm "admin/internal/identity/adapters/gorm"
	identityapplication "admin/internal/identity/application"
	"admin/internal/identity/domain"
	"admin/testsupport/testutil"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRepositoryPersistsIdentityUserWithoutLeakingGORMModel(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	user := domain.User{Username: "alice", Password: "hash", Email: "alice@example.com", Nickname: "Alice", Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if user.ID == 0 {
		t.Fatal("Create() did not return user ID")
	}
	loaded, err := repository.FindByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("FindByUsername() error = %v", err)
	}
	if loaded.ID != user.ID || loaded.Email != "alice@example.com" || loaded.Password != "hash" {
		t.Fatalf("loaded user = %#v", loaded)
	}
	exists, err := repository.ExistsByUsernameOrEmail(context.Background(), "other", "alice@example.com")
	if err != nil || !exists {
		t.Fatalf("ExistsByUsernameOrEmail() = %v, %v", exists, err)
	}
}

func TestRepositoryListsSafeDirectoryRecordsInStableOrder(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	trusted := domain.User{Username: "trusted", Password: "secret", Email: "trusted@example.com", Avatar: "https://legacy.example/trusted.png", Status: 1}
	untrusted := domain.User{Username: "untrusted", Password: "secret", Email: "untrusted@example.com", Avatar: "https://legacy.example/untrusted.png", Status: 1}
	if err := repository.Create(context.Background(), &trusted); err != nil {
		t.Fatalf("create trusted user: %v", err)
	}
	if err := repository.Create(context.Background(), &untrusted); err != nil {
		t.Fatalf("create untrusted user: %v", err)
	}
	validatedAt := time.Now().UTC()
	if _, err := repository.UpdateAvatar(context.Background(), trusted.ID, identityapplication.AvatarUpdate{
		ObjectName:  "avatars/" + strconv.FormatUint(uint64(trusted.ID), 10) + "/00000000-0000-4000-8000-000000000009.png",
		ContentType: "image/png", ContentSHA256: "hash", ValidationStatus: domain.AvatarValidationStatusValidated, ValidatedAt: &validatedAt,
	}); err != nil {
		t.Fatalf("trust avatar: %v", err)
	}

	records, err := repository.ListUsersByIDs(context.Background(), []uint{untrusted.ID, trusted.ID})
	if err != nil {
		t.Fatalf("ListUsersByIDs() error = %v", err)
	}
	if len(records) != 2 || records[0].ID != trusted.ID || !records[0].AvatarTrusted || records[1].ID != untrusted.ID || records[1].AvatarTrusted {
		t.Fatalf("directory records = %#v", records)
	}
	userIDs, err := repository.ListUserIDs(context.Background())
	if err != nil || len(userIDs) != 2 || userIDs[0] != trusted.ID || userIDs[1] != untrusted.ID {
		t.Fatalf("directory user IDs = %#v, error = %v", userIDs, err)
	}
}

func TestRepositoryConfirmsPendingEmailAndConsumesCredentialOnce(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}, identitymodule.EmailVerificationCredential{}); err != nil {
		t.Fatalf("migrate email models: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	user := domain.User{Username: "alice", Password: "hash", Email: "old@example.com", PendingEmail: "new@example.com", Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	rawToken := make([]byte, 32)
	for index := range rawToken {
		rawToken[index] = byte(index + 1)
	}
	digest := sha256.Sum256(rawToken)
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	credential := identitymodule.EmailVerificationCredential{UserID: user.ID, Email: "new@example.com", TokenHash: hex.EncodeToString(digest[:]), ExpiresAt: verifiedAt.Add(15 * time.Minute)}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	service := identityapplication.NewEmailVerificationService(repository, nil, identityapplication.WithEmailVerificationClock(func() time.Time { return verifiedAt }))
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	if err := service.Confirm(context.Background(), user.ID, token); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	loaded, err := repository.FindByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load confirmed user: %v", err)
	}
	if loaded.Email != "new@example.com" || loaded.PendingEmail != "" || loaded.EmailVerifiedAt == nil || !loaded.EmailVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("confirmed user = %#v", loaded)
	}
	if err := service.Confirm(context.Background(), user.ID, token); err == nil {
		t.Fatal("second Confirm() succeeded for a consumed credential")
	} else if code, _ := identityapplication.CodeOf(err); code != identityapplication.CodeEmailVerificationInvalid {
		t.Fatalf("second Confirm() error = %v, want invalid credential", err)
	}
}

func TestRepositoryConfirmsOccupiedPendingEmailAsConflictAndConsumesToken(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}, identitymodule.EmailVerificationCredential{}); err != nil {
		t.Fatalf("migrate email models: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	first := domain.User{Username: "first", Password: "hash", Email: "first@example.com", PendingEmail: "candidate@example.com", Role: "user", Status: 1}
	second := domain.User{Username: "second", Password: "hash", Email: "candidate@example.com", Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &first); err != nil {
		t.Fatalf("create first user: %v", err)
	}
	if err := repository.Create(context.Background(), &second); err != nil {
		t.Fatalf("create second user: %v", err)
	}
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	credential := identitymodule.EmailVerificationCredential{UserID: first.ID, Email: "candidate@example.com", TokenHash: strings.Repeat("a", 64), ExpiresAt: verifiedAt.Add(15 * time.Minute)}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	service := identityapplication.NewEmailVerificationService(repository, nil, identityapplication.WithEmailVerificationClock(func() time.Time { return verifiedAt }))
	tokenBytes := make([]byte, 32)
	for index := range tokenBytes {
		tokenBytes[index] = 0x42
	}
	digest := sha256.Sum256(tokenBytes)
	if err := db.Model(&identitymodule.EmailVerificationCredential{}).Where("id = ?", credential.ID).Update("token_hash", hex.EncodeToString(digest[:])).Error; err != nil {
		t.Fatalf("update token hash: %v", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	if err := service.Confirm(context.Background(), first.ID, token); err == nil {
		t.Fatal("Confirm() succeeded for an occupied candidate email")
	} else if code, _ := identityapplication.CodeOf(err); code != identityapplication.CodeConflict {
		t.Fatalf("Confirm() error code = %q, want conflict", code)
	}
	loaded, err := repository.FindByID(context.Background(), first.ID)
	if err != nil || loaded.Email != "first@example.com" || loaded.PendingEmail != "candidate@example.com" {
		t.Fatalf("conflict changed first user = %#v error=%v", loaded, err)
	}
}

func TestRepositoryConfirmsCredentialOnlyOnceUnderConcurrentConfirmation(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}, identitymodule.EmailVerificationCredential{}); err != nil {
		t.Fatalf("migrate email models: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	user := domain.User{Username: "concurrent", Password: "hash", Email: "old@example.com", PendingEmail: "new@example.com", Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	rawToken := bytes.Repeat([]byte{0x24}, 32)
	digest := sha256.Sum256(rawToken)
	verifiedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := db.Create(&identitymodule.EmailVerificationCredential{UserID: user.ID, Email: "new@example.com", TokenHash: hex.EncodeToString(digest[:]), ExpiresAt: verifiedAt.Add(15 * time.Minute)}).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	services := []*identityapplication.EmailVerificationService{
		identityapplication.NewEmailVerificationService(repository, nil, identityapplication.WithEmailVerificationClock(func() time.Time { return verifiedAt })),
		identityapplication.NewEmailVerificationService(repository, nil, identityapplication.WithEmailVerificationClock(func() time.Time { return verifiedAt })),
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	results := make(chan error, 2)
	for _, service := range services {
		go func(service *identityapplication.EmailVerificationService) {
			results <- service.Confirm(context.Background(), user.ID, token)
		}(service)
	}
	var success, invalid int
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if code, _ := identityapplication.CodeOf(err); code == identityapplication.CodeEmailVerificationInvalid {
			invalid++
		} else {
			t.Fatalf("concurrent Confirm() error = %v", err)
		}
	}
	if success != 1 || invalid != 1 {
		t.Fatalf("concurrent confirmations = success:%d invalid:%d, want one each", success, invalid)
	}
}

func TestRepositoryReplacesAndInvalidatesOldEmailCredential(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}, identitymodule.EmailVerificationCredential{}); err != nil {
		t.Fatalf("migrate email models: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	user := domain.User{Username: "replace", Password: "hash", Email: "old@example.com", Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	oldBytes := bytes.Repeat([]byte{0x11}, 32)
	oldDigest := sha256.Sum256(oldBytes)
	if err := db.Create(&identitymodule.EmailVerificationCredential{UserID: user.ID, Email: user.Email, TokenHash: hex.EncodeToString(oldDigest[:]), ExpiresAt: time.Now().UTC().Add(time.Hour)}).Error; err != nil {
		t.Fatalf("create old credential: %v", err)
	}
	service := identityapplication.NewEmailVerificationService(repository, nil, identityapplication.WithEmailVerificationRandom(bytes.NewReader(bytes.Repeat([]byte{0x22}, 32))))
	if _, err := service.Issue(context.Background(), user.ID, "new@example.com"); err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	oldToken := base64.RawURLEncoding.EncodeToString(oldBytes)
	if err := service.Confirm(context.Background(), user.ID, oldToken); err == nil {
		t.Fatal("old email credential remained usable after replacement")
	} else if code, _ := identityapplication.CodeOf(err); code != identityapplication.CodeEmailVerificationInvalid {
		t.Fatalf("old credential error code = %q, want invalid", code)
	}
}

func TestRepositoryPhysicallyCleansTerminalEmailCredentialsAfterRetention(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(identitygorm.UserModel{}, identitymodule.EmailVerificationCredential{}); err != nil {
		t.Fatalf("migrate email models: %v", err)
	}
	repository := identitygorm.NewRepository(db)
	user := domain.User{Username: "cleanup", Password: "hash", Email: "cleanup@example.com", Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Date(2026, time.January, 3, 3, 4, 5, 0, time.UTC)
	old := now.Add(-25 * time.Hour)
	recent := now.Add(-23 * time.Hour)
	oldCredential := identitymodule.EmailVerificationCredential{UserID: user.ID, Email: user.Email, TokenHash: strings.Repeat("b", 64), ExpiresAt: old, UsedAt: &old}
	recentCredential := identitymodule.EmailVerificationCredential{UserID: user.ID, Email: user.Email, TokenHash: strings.Repeat("c", 64), ExpiresAt: recent, UsedAt: &recent}
	if err := db.Create(&oldCredential).Error; err != nil {
		t.Fatalf("create old credential: %v", err)
	}
	if err := db.Create(&recentCredential).Error; err != nil {
		t.Fatalf("create recent credential: %v", err)
	}
	if err := repository.DeleteTerminalBefore(context.Background(), now.Add(-24*time.Hour)); err != nil {
		t.Fatalf("DeleteTerminalBefore() error = %v", err)
	}
	if err := db.Unscoped().First(&identitymodule.EmailVerificationCredential{}, oldCredential.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("old credential still exists, error = %v", err)
	}
	if err := db.Unscoped().First(&identitymodule.EmailVerificationCredential{}, recentCredential.ID).Error; err != nil {
		t.Fatalf("recent terminal credential was cleaned: %v", err)
	}
}
