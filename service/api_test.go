package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"admin/dto"
	"admin/global"
	"admin/model"
)

func TestSyncAPIsGeneratesFileAccessPermissionsAndKeepsAvatarRoutesPublic(
	t *testing.T,
) {
	db := setupAPITestDB(t)
	routes := []dto.SyncAPIItem{
		{Method: "GET", Path: "/api/admin/files/:id/download"},
		{Method: "GET", Path: "/api/admin/files/:id/preview"},
		{Method: "POST", Path: "/api/admin/files/:id/revalidate"},
		{Method: "GET", Path: "/api/avatars/:user_id"},
		{Method: "GET", Path: "/api/avatars/default"},
	}

	if _, err := SyncAPIs(routes); err != nil {
		t.Fatalf("sync APIs failed: %v", err)
	}
	if _, _, err := SyncAPIPermissions(); err != nil {
		t.Fatalf("sync API permissions failed: %v", err)
	}

	expected := map[string]string{
		"GET /api/admin/files/:id/download":    "admin.files.id.download.get",
		"GET /api/admin/files/:id/preview":     "admin.files.id.preview.get",
		"POST /api/admin/files/:id/revalidate": "admin.files.id.revalidate.post",
	}
	for route, permissionCode := range expected {
		method, path := splitTestRoute(t, route)
		var api model.API
		if err := db.Where("method = ? AND path = ?", method, path).
			First(&api).Error; err != nil {
			t.Fatalf("find synced API %s failed: %v", route, err)
		}
		if api.NeedAuth != 1 {
			t.Fatalf("%s should require authentication", route)
		}
		if api.PermissionCode != permissionCode {
			t.Fatalf(
				"%s permission code = %q, want %q",
				route,
				api.PermissionCode,
				permissionCode,
			)
		}

		var count int64
		if err := db.Model(&model.Permission{}).
			Where("code = ?", permissionCode).
			Count(&count).Error; err != nil {
			t.Fatalf("count permission %q failed: %v", permissionCode, err)
		}
		if count != 1 {
			t.Fatalf("permission %q count = %d, want 1", permissionCode, count)
		}
	}

	for _, route := range []string{
		"GET /api/avatars/:user_id",
		"GET /api/avatars/default",
	} {
		method, path := splitTestRoute(t, route)
		var api model.API
		if err := db.Where("method = ? AND path = ?", method, path).
			First(&api).Error; err != nil {
			t.Fatalf("find synced API %s failed: %v", route, err)
		}
		if api.NeedAuth != 0 || api.PermissionCode != "" {
			t.Fatalf(
				"%s should remain public, got need_auth=%d permission_code=%q",
				route,
				api.NeedAuth,
				api.PermissionCode,
			)
		}
	}
}

func setupAPITestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := global.DB
	t.Cleanup(func() {
		global.DB = previousDB
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&model.API{}, &model.Permission{}); err != nil {
		t.Fatalf("auto migrate API tables failed: %v", err)
	}
	global.DB = db
	return db
}

func splitTestRoute(t *testing.T, route string) (string, string) {
	t.Helper()
	for index, value := range route {
		if value == ' ' {
			return route[:index], route[index+1:]
		}
	}
	t.Fatalf("invalid route %q", route)
	return "", ""
}
