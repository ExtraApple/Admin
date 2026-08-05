package gormadapter_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"admin/testsupport/testutil"
	"admin/internal/audit"
	gormadapter "admin/internal/audit/adapters/gorm"
)

func TestRepositoryRecordsQueriesAndArchivesAuditLogs(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(audit.Models()...); err != nil {
		t.Fatalf("migrate Audit models: %v", err)
	}
	repository := gormadapter.NewRepository(db)
	ctx := context.Background()

	oldTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	logs := []audit.AuditLog{
		{UserID: 7, Username: "alice", Method: "POST", Path: "/api/admin/permissions", Metadata: json.RawMessage(`{"purpose":"managed_file"}`), Status: 200, Category: audit.AuditCategoryPermission, CreatedAt: oldTime},
		{UserID: 7, Username: "alice", Method: "GET", Path: "/api/admin/files", Status: 200, Category: audit.AuditCategoryDataAccess, CreatedAt: newTime},
	}
	for index := range logs {
		if err := repository.Create(ctx, &logs[index]); err != nil {
			t.Fatalf("create audit log %d: %v", index, err)
		}
	}
	listed, total, err := repository.List(ctx, audit.AuditLogListRequest{Page: 1, Size: 10, UserID: 7}, []string{audit.AuditCategoryPermission})
	if err != nil || total != 1 || len(listed) != 1 || listed[0].Path != "/api/admin/permissions" || string(listed[0].Metadata) != `{"purpose":"managed_file"}` {
		t.Fatalf("listed logs = %#v total=%d err=%v", listed, total, err)
	}
	expired, err := repository.Expired(ctx, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 100)
	if err != nil || len(expired) != 1 || expired[0].ID != logs[0].ID {
		t.Fatalf("expired logs = %#v err=%v", expired, err)
	}
	archivedAt := time.Date(2026, 8, 4, 13, 0, 0, 0, time.UTC)
	archives := []audit.AuditLogArchive{audit.ToArchive(expired[0], archivedAt)}
	if err := repository.Archive(ctx, archives, []uint{expired[0].ID}); err != nil {
		t.Fatalf("archive logs: %v", err)
	}
	var hotCount, archiveCount int64
	if err := db.Model(&audit.AuditLog{}).Count(&hotCount).Error; err != nil {
		t.Fatalf("count hot logs: %v", err)
	}
	if err := db.Model(&audit.AuditLogArchive{}).Count(&archiveCount).Error; err != nil {
		t.Fatalf("count archived logs: %v", err)
	}
	if hotCount != 1 || archiveCount != 1 {
		t.Fatalf("hot=%d archive=%d", hotCount, archiveCount)
	}
}
