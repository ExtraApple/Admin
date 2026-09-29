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

func TestMySQLCleanupRetryCountConstraintAndVersionGate(t *testing.T) {
	dsn := os.Getenv("ADMIN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("ADMIN_TEST_MYSQL_DSN is required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var version string
	if err := db.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(db); err != nil {
		t.Fatalf("MySQL %s migration rejected supported server: %v", version, err)
	}
	fileID := uint(time.Now().UnixNano() % 1000000000)
	for _, retryCount := range []uint{0, 24} {
		job := files.MessageImageCleanupJob{FileID: fileID + retryCount, Bucket: "probe", ObjectName: fmt.Sprintf("message-images/retry-%d.png", retryCount), ObjectKeyHash: fmt.Sprintf("%064x", fileID+retryCount), Status: "pending", RetryCount: retryCount}
		if err := db.Create(&job).Error; err != nil {
			t.Fatalf("MySQL %s rejected retry_count=%d: %v", version, retryCount, err)
		}
		t.Cleanup(func() {
			if err := db.Delete(&files.MessageImageCleanupJob{}, job.ID).Error; err != nil {
				t.Error(err)
			}
		})
	}
	if err := db.Exec("INSERT INTO message_image_cleanup_jobs (file_id, bucket, object_name, object_key_hash, status, retry_count) VALUES (?, ?, ?, ?, ?, ?)", fileID+25, "probe", "message-images/retry-invalid.png", strings.Repeat("d", 64), "pending", 25).Error; err == nil {
		t.Fatalf("MySQL %s accepted retry_count=25", version)
	}
}

func TestMySQLMigrateRejectsLegacyOutOfRangeCleanupRetryCount(t *testing.T) {
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
	name := fmt.Sprintf("c5_retry_invalid_%d", time.Now().UnixNano())
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
	file := files.File{Bucket: "probe", ObjectName: "message-images/legacy-invalid.png", Purpose: "message_image"}
	if err := db.Create(&file).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE message_image_cleanup_jobs (id bigint unsigned NOT NULL AUTO_INCREMENT, file_id bigint unsigned NOT NULL, bucket varchar(100) NOT NULL, object_name varchar(500) NOT NULL, object_key_hash char(64) NOT NULL, status varchar(16) NOT NULL, retry_count bigint unsigned NOT NULL, next_retry_at datetime(3) NULL, last_error_code varchar(100) NULL, created_at datetime(3) NULL, updated_at datetime(3) NULL, PRIMARY KEY (id), UNIQUE KEY ux_file_id (file_id), UNIQUE KEY ux_path (object_key_hash))").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO message_image_cleanup_jobs (file_id, bucket, object_name, object_key_hash, status, retry_count) VALUES (?, ?, ?, ?, ?, ?)", file.ID, file.Bucket, file.ObjectName, strings.Repeat("e", 64), "pending", 25).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(db); err == nil {
		t.Fatal("migration accepted legacy retry_count=25")
	}
	var retained files.MessageImageCleanupJob
	if err := db.Where("file_id = ?", file.ID).First(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if retained.RetryCount != 25 || retained.Bucket != file.Bucket || retained.ObjectName != file.ObjectName {
		t.Fatalf("migration changed invalid cleanup location: %+v", retained)
	}
	var retainedFile files.File
	if err := db.First(&retainedFile, file.ID).Error; err != nil || retainedFile.Bucket != file.Bucket || retainedFile.ObjectName != file.ObjectName {
		t.Fatalf("migration changed the File Record: %+v, %v", retainedFile, err)
	}
	// Without proof the last storage attempt was not made, an operator repairs to 24; migration never does.
	if err := db.Table("message_image_cleanup_jobs").Where("file_id = ?", file.ID).Update("retry_count", 24).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.Migrate(db); err != nil {
		t.Fatalf("migration cannot resume after operator repair: %v", err)
	}
	if err := db.Where("file_id = ?", file.ID).First(&retained).Error; err != nil || retained.RetryCount != 24 || retained.Bucket != file.Bucket || retained.ObjectName != file.ObjectName {
		t.Fatalf("migration changed repaired cleanup location: %+v, %v", retained, err)
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
