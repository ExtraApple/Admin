package gormadapter_test

import (
	"context"
	"testing"

	"admin/testsupport/testutil"
	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/identity/domain"
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
