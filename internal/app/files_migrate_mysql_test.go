//go:build mysql_integration

package app_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"admin/internal/app"
	"admin/internal/files"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMySQLMessageImageCleanupMigration(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{NowFunc: func() time.Time { return time.Now().UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if err := app.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var ids []uint
	var jobIDs []uint
	t.Cleanup(func() {
		if len(jobIDs) > 0 {
			if err := db.Delete(&files.MessageImageCleanupJob{}, jobIDs).Error; err != nil {
				t.Error(err)
			}
		}
		if len(ids) > 0 {
			if err := db.Unscoped().Delete(&files.File{}, ids).Error; err != nil {
				t.Error(err)
			}
		}
	})
	for _, input := range []struct{ purpose, mime, before, after string }{
		{"message_image", "image/png", "validated", "validated"},
		{"message_image", "image/webp", "legacy_unverified", "legacy_unverified"},
		{"managed_file", "image/png", "validated", "legacy_unverified"},
		{"managed_file", "text/plain", "validated", "validated"},
		{"", "image/png", "validated", "legacy_unverified"},
	} {
		file := files.File{Bucket: "c5-probe", ObjectName: input.purpose + input.mime, Purpose: input.purpose, ContentType: input.mime, ValidationStatus: input.before}
		if err := db.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, file.ID)
		if input.purpose == "" {
			if err := db.Model(&file).Update("purpose", "").Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := app.Migrate(db); err != nil {
			t.Fatal(err)
		}
		var restored files.File
		if err := db.First(&restored, file.ID).Error; err != nil {
			t.Fatal(err)
		}
		if restored.ValidationStatus != input.after {
			t.Fatalf("purpose %q MIME %q: got %s, want %s", input.purpose, input.mime, restored.ValidationStatus, input.after)
		}
	}
	job := files.MessageImageCleanupJob{FileID: ids[0], Bucket: strings.Repeat("b", 100), ObjectName: strings.Repeat("n", 500), ObjectKeyHash: strings.Repeat("a", 64), Status: "dead", RetryCount: 24}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	jobIDs = append(jobIDs, job.ID)
	if err := app.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var restored files.MessageImageCleanupJob
	if err := db.First(&restored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if restored.Bucket != job.Bucket || restored.ObjectName != job.ObjectName || restored.Status != "dead" || restored.RetryCount != 24 {
		t.Fatalf("migration changed recovery record: %+v", restored)
	}
	for _, duplicate := range []files.MessageImageCleanupJob{
		{FileID: ids[0], Bucket: "other", ObjectName: "other", ObjectKeyHash: strings.Repeat("b", 64)},
		{FileID: ids[1], Bucket: job.Bucket, ObjectName: job.ObjectName, ObjectKeyHash: job.ObjectKeyHash},
	} {
		if err := db.Create(&duplicate).Error; err == nil {
			jobIDs = append(jobIDs, duplicate.ID)
			t.Fatal("MySQL accepted duplicate file or object cleanup")
		}
	}
	var count int64
	if err := db.Model(&files.MessageImageCleanupJob{}).Where("file_id IN ?", ids).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration created cleanup work: got %d jobs, want one explicit job", count)
	}
}

func TestMySQLMigrateLegacyNullFilePurpose(t *testing.T) {
	config, err := mysqldriver.ParseDSN(os.Getenv("ADMIN_TEST_MYSQL_DSN"))
	if err != nil || config.DBName == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN must identify a test database")
	}
	admin, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer adminSQL.Close()
	name := fmt.Sprintf("c5_null_purpose_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := admin.Exec("DROP DATABASE " + name).Error; err != nil {
			t.Error(err)
		}
	}()
	config.DBName = name
	db, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&files.File{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE files MODIFY purpose varchar(32) NULL DEFAULT 'managed_file'").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO files (bucket, object_name, purpose, content_type, validation_status) VALUES (?, ?, NULL, ?, ?)", "probe", "legacy.png", "image/png", "validated").Error; err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migrate legacy NULL purpose: %v", err)
	}
	var file files.File
	if err := db.First(&file).Error; err != nil {
		t.Fatal(err)
	}
	if file.ValidationStatus != files.FileValidationStatusLegacyUnverified {
		t.Fatalf("legacy NULL purpose bypassed ordinary file policy: %s", file.ValidationStatus)
	}
}
