package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"admin/global"
	"admin/initialize"
	"admin/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestToAuditLogArchiveCopiesStructuredMetadata(t *testing.T) {
	metadata := json.RawMessage(
		`{"purpose":"avatar","validation_result":"rejected","reason_code":"IMAGE_DECODE_INVALID"}`,
	)
	archivedAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	archive := toAuditLogArchive(model.AuditLog{
		Metadata: metadata,
	}, archivedAt)

	if string(archive.Metadata) != string(metadata) {
		t.Fatalf("archive metadata = %s, want exact copy %s", archive.Metadata, metadata)
	}
	if !archive.ArchivedAt.Equal(archivedAt) {
		t.Fatalf("archived_at = %s, want %s", archive.ArchivedAt, archivedAt)
	}
}

func TestArchiveAuditLogsKeepsHotRecordWhenArchiveInsertFails(t *testing.T) {
	db := openAuditArchiveTestDB(t)

	log := model.AuditLog{
		Method:    "POST",
		Path:      "/api/admin/files",
		Metadata:  json.RawMessage(`{"purpose":"managed_file","validation_result":"accepted"}`),
		CreatedAt: time.Now().AddDate(0, 0, -30),
	}
	if err := db.Create(&log).Error; err != nil {
		t.Fatalf("create hot audit log: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER fail_archive_insert
		BEFORE INSERT ON audit_log_archives
		BEGIN
			SELECT RAISE(ABORT, 'archive failure');
		END
	`).Error; err != nil {
		t.Fatalf("create archive failure trigger: %v", err)
	}

	conf := &initialize.Config{}
	conf.AuditLogArchive.Enabled = true
	conf.AuditLogArchive.RetentionDays = 7
	conf.AuditLogArchive.BatchSize = 10
	ArchiveAuditLogs(conf)

	var hotCount int64
	if err := db.Model(&model.AuditLog{}).Where("id = ?", log.ID).Count(&hotCount).Error; err != nil {
		t.Fatalf("count hot audit logs: %v", err)
	}
	var archiveCount int64
	if err := db.Model(&model.AuditLogArchive{}).Count(&archiveCount).Error; err != nil {
		t.Fatalf("count archived audit logs: %v", err)
	}
	if hotCount != 1 || archiveCount != 0 {
		t.Fatalf("transaction result: hot=%d archive=%d, want hot=1 archive=0", hotCount, archiveCount)
	}
}

func TestArchiveAuditLogsDisabledLeavesHotRecordsUntouched(t *testing.T) {
	db := openAuditArchiveTestDB(t)

	log := model.AuditLog{
		Method:    "POST",
		Path:      "/api/admin/files",
		Metadata:  json.RawMessage(`{"purpose":"managed_file","validation_result":"accepted"}`),
		CreatedAt: time.Now().AddDate(0, 0, -30),
	}
	if err := db.Create(&log).Error; err != nil {
		t.Fatalf("create hot audit log: %v", err)
	}

	conf := &initialize.Config{}
	conf.AuditLogArchive.Enabled = false
	conf.AuditLogArchive.RetentionDays = 7
	conf.AuditLogArchive.BatchSize = 10
	ArchiveAuditLogs(conf)

	var hotCount int64
	if err := db.Model(&model.AuditLog{}).Where("id = ?", log.ID).Count(&hotCount).Error; err != nil {
		t.Fatalf("count hot audit logs: %v", err)
	}
	var archiveCount int64
	if err := db.Model(&model.AuditLogArchive{}).Count(&archiveCount).Error; err != nil {
		t.Fatalf("count archived audit logs: %v", err)
	}
	if hotCount != 1 || archiveCount != 0 {
		t.Fatalf("disabled archive result: hot=%d archive=%d, want hot=1 archive=0",
			hotCount, archiveCount)
	}
}

func TestArchiveAuditLogsMovesExpiredRecordWithExactMetadataAfterSuccessfulCopy(t *testing.T) {
	db := openAuditArchiveTestDB(t)

	metadata := json.RawMessage(
		`{"purpose":"avatar","file_name":"portrait.png","validation_result":"rejected","reason_code":"IMAGE_DECODE_INVALID"}`,
	)
	expired := model.AuditLog{
		UserID:    42,
		Username:  "alice",
		Method:    "POST",
		Path:      "/api/user/avatar",
		Query:     "source=profile",
		Body:      "[multipart omitted]",
		Metadata:  metadata,
		Status:    422,
		Duration:  17,
		ClientIP:  "203.0.113.8",
		UserAgent: "archive-test-agent",
		Category:  AuditCategoryOperation,
		CreatedAt: time.Now().AddDate(0, 0, -30),
	}
	recent := model.AuditLog{
		Method:    "GET",
		Path:      "/api/user/info",
		Category:  AuditCategoryDataAccess,
		CreatedAt: time.Now(),
	}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatalf("create expired audit log: %v", err)
	}
	if err := db.Create(&recent).Error; err != nil {
		t.Fatalf("create recent audit log: %v", err)
	}

	conf := &initialize.Config{}
	conf.AuditLogArchive.Enabled = true
	conf.AuditLogArchive.RetentionDays = 7
	conf.AuditLogArchive.BatchSize = 10
	ArchiveAuditLogs(conf)

	var archived model.AuditLogArchive
	if err := db.Where("path = ?", expired.Path).First(&archived).Error; err != nil {
		t.Fatalf("load archived audit log: %v", err)
	}
	if archived.UserID != expired.UserID ||
		archived.Username != expired.Username ||
		archived.Method != expired.Method ||
		archived.Path != expired.Path ||
		archived.Query != expired.Query ||
		archived.Body != expired.Body ||
		string(archived.Metadata) != string(metadata) ||
		archived.Status != expired.Status ||
		archived.Duration != expired.Duration ||
		archived.ClientIP != expired.ClientIP ||
		archived.UserAgent != expired.UserAgent ||
		archived.Category != expired.Category ||
		!archived.CreatedAt.Equal(expired.CreatedAt) ||
		archived.ArchivedAt.IsZero() {
		t.Fatalf("archived record = %#v, want exact hot-record copy plus archived_at", archived)
	}

	var expiredHotCount int64
	if err := db.Model(&model.AuditLog{}).
		Where("id = ?", expired.ID).
		Count(&expiredHotCount).Error; err != nil {
		t.Fatalf("count expired hot audit log: %v", err)
	}
	var recentHotCount int64
	if err := db.Model(&model.AuditLog{}).
		Where("id = ?", recent.ID).
		Count(&recentHotCount).Error; err != nil {
		t.Fatalf("count recent hot audit log: %v", err)
	}
	if expiredHotCount != 0 || recentHotCount != 1 {
		t.Fatalf("hot-table result: expired=%d recent=%d, want 0 and 1",
			expiredHotCount, recentHotCount)
	}
}

func openAuditArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "audit-archive.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}, &model.AuditLogArchive{}); err != nil {
		t.Fatalf("migrate audit tables: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
