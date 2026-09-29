package app_test

import (
	"fmt"
	"strings"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/app"
	"admin/internal/audit"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/dictionary"
	"admin/internal/files"
	"admin/internal/identity"
	"admin/internal/navigation"
	internalorganization "admin/internal/organization"
	"admin/testsupport/testutil"
	"testing"
	"time"
)

func TestMigrateCollectsExistingModels(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate application database: %v", err)
	}

	wantModels := []any{
		identity.User{},
		authgorm.UserAccessVersion{},
		authgorm.Role{},
		authgorm.UserRole{},
		authgorm.RoleDataScope{},
		authgorm.Permission{},
		authgorm.RolePermission{},
		authgorm.PermissionGroup{},
		files.File{},
		navigation.MenuModel{},
		navigation.RoleMenuModel{},
		navigation.MenuAPIModel{},
		audit.AuditLog{},
		audit.AuditLogArchive{},
		dictionary.Type{},
		dictionary.Item{},
		internalorganization.Unit{},
		internalorganization.Membership{},
		apigorm.API{},
	}
	for _, registeredModel := range wantModels {
		if !db.Migrator().HasTable(registeredModel) {
			t.Fatalf("migration did not create table for %T", registeredModel)
		}
	}
}

func TestMigrateBackfillsLegacyMembershipJoinedAt(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.Exec("CREATE TABLE user_organizations (user_id integer NOT NULL, organization_id integer NOT NULL, PRIMARY KEY (user_id, organization_id))").Error; err != nil {
		t.Fatalf("create legacy memberships: %v", err)
	}
	if err := db.Exec("INSERT INTO user_organizations (user_id, organization_id) VALUES (?, ?)", 7, 10).Error; err != nil {
		t.Fatalf("insert legacy membership: %v", err)
	}
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate application database: %v", err)
	}
	var membership internalorganization.Membership
	if err := db.First(&membership, "user_id = ? AND organization_id = ?", 7, 10).Error; err != nil {
		t.Fatalf("load migrated membership: %v", err)
	}
	if membership.CreatedAt.IsZero() || membership.CreatedAt.Before(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("legacy membership joined at = %s", membership.CreatedAt)
	}
}

func TestMigrateCreatesRecoverableMessageImageCleanupQueue(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatal(err)
	}
	job := files.MessageImageCleanupJob{FileID: 71, Bucket: "files", ObjectName: "message-images/old.png", ObjectKeyHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: "dead", RetryCount: 24}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("persist cleanup after application migration: %v", err)
	}
	if err := app.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var retained files.MessageImageCleanupJob
	if err := db.First(&retained, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if retained.Status != "dead" || retained.RetryCount != 24 || retained.ObjectName != job.ObjectName {
		t.Fatalf("repeated migration changed recovery state: %+v", retained)
	}
	duplicate := files.MessageImageCleanupJob{FileID: 72, Bucket: job.Bucket, ObjectName: job.ObjectName, ObjectKeyHash: job.ObjectKeyHash}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("application migration permits duplicate cleanup location")
	}
}

func TestMigrateConstrainsMessageImageCleanupRetryCount(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, retryCount := range []uint{0, 24} {
		job := files.MessageImageCleanupJob{FileID: retryCount + 1, Bucket: "files", ObjectName: fmt.Sprintf("message-images/%d.png", retryCount), ObjectKeyHash: fmt.Sprintf("%064x", retryCount+1), Status: "pending", RetryCount: retryCount}
		if err := db.Create(&job).Error; err != nil {
			t.Fatalf("retry_count %d should satisfy the database constraint: %v", retryCount, err)
		}
	}
	for _, retryCount := range []int{-1, 25} {
		err := db.Exec("INSERT INTO message_image_cleanup_jobs (file_id, bucket, object_name, object_key_hash, status, retry_count) VALUES (?, ?, ?, ?, ?, ?)", uint(retryCount+10), "files", "message-images/invalid.png", fmt.Sprintf("%064x", retryCount+10), "pending", retryCount).Error
		if err == nil {
			t.Fatalf("retry_count %d bypassed the database constraint", retryCount)
		}
	}
}

func TestMigrateRejectsLegacyOutOfRangeCleanupRetryCount(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.Exec("CREATE TABLE message_image_cleanup_jobs (id integer primary key, file_id integer not null, bucket text not null, object_name text not null, object_key_hash text not null, status text not null, retry_count integer not null, next_retry_at datetime, last_error_code text, created_at datetime, updated_at datetime)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO message_image_cleanup_jobs (file_id, bucket, object_name, object_key_hash, status, retry_count) VALUES (?, ?, ?, ?, ?, ?)", 7, "files", "message-images/legacy.png", strings.Repeat("a", 64), "pending", 25).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(db); err == nil {
		t.Fatal("migration accepted a legacy retry_count above 24")
	}
	var retryCount int
	if err := db.Table("message_image_cleanup_jobs").Select("retry_count").Where("file_id = ?", 7).Scan(&retryCount).Error; err != nil {
		t.Fatal(err)
	}
	if retryCount != 25 {
		t.Fatalf("migration changed invalid legacy retry_count to %d", retryCount)
	}
}

func TestMigratePreservesValidatedMessageImages(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := app.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{"message_image", "managed_file"} {
		file := files.File{Bucket: "files", ObjectName: purpose + ".png", Purpose: purpose, ContentType: "image/png", ValidationStatus: files.FileValidationStatusValidated}
		if err := db.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := app.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var records []files.File
	if err := db.Order("id").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ValidationStatus != files.FileValidationStatusValidated || records[1].ValidationStatus != files.FileValidationStatusLegacyUnverified {
		t.Fatalf("migration failed to isolate ordinary file policy: %+v", records)
	}
}
