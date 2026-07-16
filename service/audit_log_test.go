package service

import (
	"encoding/json"
	"testing"
	"time"

	"admin/dto"
	"admin/global"
	"admin/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestListAuditLogsReturnsStructuredMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	metadata := json.RawMessage(`{"purpose":"managed_file","validation_result":"validated"}`)
	if err := db.Create(&model.AuditLog{
		Method:   "POST",
		Path:     "/api/admin/files",
		Metadata: metadata,
	}).Error; err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	logs, total, err := ListAuditLogs(dto.AuditLogListReq{})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("unexpected result size: total=%d len=%d", total, len(logs))
	}
	if string(logs[0].Metadata) != string(metadata) {
		t.Fatalf("metadata: got %s, want %s", logs[0].Metadata, metadata)
	}
}

func TestListAuditLogsByCategoriesReturnsEndpointSpecificResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	baseTime := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	logs := []model.AuditLog{
		{Path: "/api/other", Category: AuditCategoryAPI, CreatedAt: baseTime},
		{Path: "/api/login", Category: AuditCategoryLogin, CreatedAt: baseTime.Add(time.Minute)},
		{Path: "/api/admin/files", Category: AuditCategoryOperation, CreatedAt: baseTime.Add(2 * time.Minute)},
		{Path: "/api/admin/roles/7/permissions", Category: AuditCategoryPermission, CreatedAt: baseTime.Add(3 * time.Minute)},
		{Path: "/api/admin/users", Category: AuditCategoryDataAccess, CreatedAt: baseTime.Add(4 * time.Minute)},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("create audit logs: %v", err)
	}

	tests := []struct {
		name       string
		categories []string
		want       map[string]bool
	}{
		{
			name:       "login endpoint",
			categories: []string{AuditCategoryLogin},
			want:       map[string]bool{AuditCategoryLogin: true},
		},
		{
			name:       "operation endpoint includes permission mutations",
			categories: []string{AuditCategoryOperation, AuditCategoryPermission},
			want: map[string]bool{
				AuditCategoryOperation:  true,
				AuditCategoryPermission: true,
			},
		},
		{
			name:       "permission endpoint",
			categories: []string{AuditCategoryPermission},
			want:       map[string]bool{AuditCategoryPermission: true},
		},
		{
			name:       "data access endpoint",
			categories: []string{AuditCategoryDataAccess},
			want:       map[string]bool{AuditCategoryDataAccess: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, total, err := ListAuditLogsByCategories(
				dto.AuditLogListReq{Page: 1, Size: 10},
				tt.categories...,
			)
			if err != nil {
				t.Fatalf("ListAuditLogsByCategories() error = %v", err)
			}
			if total != int64(len(tt.want)) || len(result) != len(tt.want) {
				t.Fatalf("result total=%d len=%d, want %d", total, len(result), len(tt.want))
			}
			for _, item := range result {
				if !tt.want[item.Category] {
					t.Fatalf("unexpected category %q in %#v", item.Category, result)
				}
			}
		})
	}

	all, total, err := ListAuditLogs(dto.AuditLogListReq{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if total != int64(len(logs)) || len(all) != len(logs) {
		t.Fatalf("ListAuditLogs() total=%d len=%d, want %d", total, len(all), len(logs))
	}
}
