package gormadapter_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	identitygorm "admin/internal/identity/adapters/gorm"
	identityapplication "admin/internal/identity/application"
	"admin/internal/identity/domain"
	"admin/testsupport/testutil"
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
