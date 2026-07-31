package model_test

import (
	"errors"
	"strings"
	"testing"

	"admin/initialize/testutil"
	"admin/model"

	"gorm.io/gorm"
)

func TestUserAccessVersionPersistsPositiveVersionByUserID(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)

	if err := db.AutoMigrate(&model.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate user access version: %v", err)
	}

	record := model.UserAccessVersion{
		UserID:  42,
		Version: 3,
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create user access version: %v", err)
	}

	var stored model.UserAccessVersion
	if err := db.First(&stored, "user_id = ?", record.UserID).Error; err != nil {
		t.Fatalf("read user access version: %v", err)
	}
	if stored.UserID != record.UserID {
		t.Fatalf("user id: got %d, want %d", stored.UserID, record.UserID)
	}
	if stored.Version != record.Version {
		t.Fatalf("version: got %d, want %d", stored.Version, record.Version)
	}
	if stored.CreatedAt.IsZero() {
		t.Fatal("created_at should be populated")
	}
	if stored.UpdatedAt.IsZero() {
		t.Fatal("updated_at should be populated")
	}

	duplicate := model.UserAccessVersion{
		UserID:  record.UserID,
		Version: record.Version + 1,
	}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("duplicate user_id should violate the primary key")
	}

	if err := db.Exec(
		"INSERT INTO user_access_versions (user_id, version, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		record.UserID+1,
		0,
	).Error; err == nil {
		t.Fatal("version 0 should violate the positive-version constraint")
	}
}

func TestUserAccessVersionSchemaHasNoSoftDeleteOrCascade(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)

	if err := db.AutoMigrate(&model.User{}, &model.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate user and access version: %v", err)
	}
	if db.Migrator().HasColumn(&model.UserAccessVersion{}, "deleted_at") {
		t.Fatal("user_access_versions must not contain deleted_at")
	}

	type foreignKey struct {
		Table    string `gorm:"column:table"`
		OnDelete string `gorm:"column:on_delete"`
	}
	var foreignKeys []foreignKey
	if err := db.Raw("PRAGMA foreign_key_list('user_access_versions')").Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("inspect user access version foreign keys: %v", err)
	}
	for _, key := range foreignKeys {
		if strings.EqualFold(key.OnDelete, "CASCADE") {
			t.Fatalf("foreign key to %q must not use ON DELETE CASCADE", key.Table)
		}
	}

	user := model.User{
		Username: "soft-deleted-user",
		Password: "secret",
		Email:    "soft-deleted-user@example.com",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	version := model.UserAccessVersion{
		UserID:  user.ID,
		Version: 2,
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create user access version: %v", err)
	}
	if err := db.Delete(&user).Error; err != nil {
		t.Fatalf("soft delete user: %v", err)
	}

	var stored model.UserAccessVersion
	if err := db.First(&stored, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("access version should remain after user soft delete: %v", err)
	}
	if stored.Version != version.Version {
		t.Fatalf("retained version: got %d, want %d", stored.Version, version.Version)
	}
}

func TestModelsAutoMigrateAccessVersionStorage(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)

	if err := db.AutoMigrate(model.Models...); err != nil {
		t.Fatalf("auto migrate registered models: %v", err)
	}

	if !db.Migrator().HasTable(&model.UserAccessVersion{}) {
		t.Fatal("registered UserAccessVersion model was not auto-migrated")
	}
	if db.Migrator().HasTable("access_version_migration_states") {
		t.Fatal("retired access_version_migration_states table was auto-migrated")
	}
}

func TestUserAccessVersionSQLiteDefaultsAndTransactions(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)

	if err := db.AutoMigrate(&model.UserAccessVersion{}); err != nil {
		t.Fatalf("migrate user access version: %v", err)
	}

	defaulted := model.UserAccessVersion{UserID: 201}
	if err := db.Create(&defaulted).Error; err != nil {
		t.Fatalf("create defaulted user access version: %v", err)
	}
	var storedDefault model.UserAccessVersion
	if err := db.First(&storedDefault, "user_id = ?", defaulted.UserID).Error; err != nil {
		t.Fatalf("read defaulted user access version: %v", err)
	}
	if storedDefault.Version != 1 {
		t.Fatalf("default version: got %d, want 1", storedDefault.Version)
	}

	committed := model.UserAccessVersion{UserID: 202, Version: 4}
	commitTx := db.Begin()
	if commitTx.Error != nil {
		t.Fatalf("begin commit transaction: %v", commitTx.Error)
	}
	if err := commitTx.Create(&committed).Error; err != nil {
		_ = commitTx.Rollback().Error
		t.Fatalf("create committed user access version: %v", err)
	}
	if err := commitTx.Commit().Error; err != nil {
		t.Fatalf("commit user access version: %v", err)
	}
	if err := db.First(&model.UserAccessVersion{}, "user_id = ?", committed.UserID).Error; err != nil {
		t.Fatalf("committed user access version should be visible: %v", err)
	}

	rolledBack := model.UserAccessVersion{UserID: 203, Version: 5}
	rollbackTx := db.Begin()
	if rollbackTx.Error != nil {
		t.Fatalf("begin rollback transaction: %v", rollbackTx.Error)
	}
	if err := rollbackTx.Create(&rolledBack).Error; err != nil {
		_ = rollbackTx.Rollback().Error
		t.Fatalf("create rolled-back user access version: %v", err)
	}
	if err := rollbackTx.Rollback().Error; err != nil {
		t.Fatalf("rollback user access version: %v", err)
	}
	err := db.First(&model.UserAccessVersion{}, "user_id = ?", rolledBack.UserID).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("rolled-back user access version lookup: got %v, want record not found", err)
	}
}
