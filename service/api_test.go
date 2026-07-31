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

func TestSyncAPIsKeepsRefreshRoutePublic(t *testing.T) {
	db := setupAPITestDB(t)
	if _, err := SyncAPIs([]dto.SyncAPIItem{
		{Method: "POST", Path: "/api/refresh"},
	}); err != nil {
		t.Fatalf("sync refresh API failed: %v", err)
	}
	if _, _, err := SyncAPIPermissions(); err != nil {
		t.Fatalf("sync refresh API permission failed: %v", err)
	}

	var api model.API
	if err := db.Where("method = ? AND path = ?", "POST", "/api/refresh").
		First(&api).Error; err != nil {
		t.Fatalf("find synced refresh API failed: %v", err)
	}
	if api.NeedAuth != 0 || api.PermissionCode != "" {
		t.Fatalf(
			"POST /api/refresh should remain public, got need_auth=%d permission_code=%q",
			api.NeedAuth,
			api.PermissionCode,
		)
	}

	var permissionCount int64
	if err := db.Model(&model.Permission{}).
		Where("code = ?", "refresh.post").
		Count(&permissionCount).Error; err != nil {
		t.Fatalf("count refresh permission failed: %v", err)
	}
	if permissionCount != 0 {
		t.Fatalf("refresh.post permission count = %d, want 0", permissionCount)
	}
}

func TestSyncAPIsRepairsExistingRefreshRouteMetadata(t *testing.T) {
	db := setupAPITestDB(t)
	stale := model.API{
		Name:           "POST /api/refresh",
		Method:         "POST",
		Path:           "/api/refresh",
		PermissionCode: "refresh.post",
		Status:         1,
		NeedAuth:       1,
		NeedAudit:      1,
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("create stale refresh API metadata: %v", err)
	}

	if _, err := SyncAPIs([]dto.SyncAPIItem{
		{Method: "POST", Path: "/api/refresh"},
	}); err != nil {
		t.Fatalf("resync existing refresh API failed: %v", err)
	}
	if _, _, err := SyncAPIPermissions(); err != nil {
		t.Fatalf("resync existing refresh API permission failed: %v", err)
	}

	var repaired model.API
	if err := db.First(&repaired, stale.ID).Error; err != nil {
		t.Fatalf("read repaired refresh API metadata: %v", err)
	}
	if repaired.NeedAuth != 0 || repaired.PermissionCode != "" {
		t.Fatalf(
			"existing POST /api/refresh metadata was not repaired: need_auth=%d permission_code=%q",
			repaired.NeedAuth,
			repaired.PermissionCode,
		)
	}

	var permissionCount int64
	if err := db.Model(&model.Permission{}).
		Where("code = ?", "refresh.post").
		Count(&permissionCount).Error; err != nil {
		t.Fatalf("count repaired refresh permission failed: %v", err)
	}
	if permissionCount != 0 {
		t.Fatalf("refresh.post permission count after repair = %d, want 0", permissionCount)
	}
}

func TestSyncAPIsLeavesOtherExistingPublicMetadataUnchanged(t *testing.T) {
	db := setupAPITestDB(t)
	existing := model.API{
		Name:           "POST /api/login",
		Method:         "POST",
		Path:           "/api/login",
		PermissionCode: "custom.login",
		Status:         1,
		NeedAuth:       1,
		NeedAudit:      1,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing login API metadata: %v", err)
	}

	if _, err := SyncAPIs([]dto.SyncAPIItem{
		{Method: "POST", Path: "/api/login"},
	}); err != nil {
		t.Fatalf("resync existing login API failed: %v", err)
	}

	var stored model.API
	if err := db.First(&stored, existing.ID).Error; err != nil {
		t.Fatalf("read existing login API metadata: %v", err)
	}
	if stored.NeedAuth != existing.NeedAuth ||
		stored.PermissionCode != existing.PermissionCode {
		t.Fatalf(
			"unrelated public metadata changed: need_auth=%d permission_code=%q",
			stored.NeedAuth,
			stored.PermissionCode,
		)
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
