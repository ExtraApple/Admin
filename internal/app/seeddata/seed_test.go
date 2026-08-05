package seeddata_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	"admin/internal/apimetadata/application"
	"admin/internal/app"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/dictionary"
	"admin/internal/identity"
	"admin/internal/navigation"
	platformconfig "admin/internal/platform/config"
	"admin/internal/routecatalog"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRunInitializesRequiredData(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("run seed failed: %v", err)
	}

	assertCount(t, db.Model(&authgorm.Role{}).Where("code IN ?", []string{"admin", "user"}), 2, "default roles")
	assertCount(t, db.Model(&authgorm.PermissionGroup{}).Where("name IN ?", []string{"auth", "user", "role", "permission", "menu", "api", "organization", "dict", "file", "audit", "system"}), 11, "permission groups")
	assertCount(t, db.Model(&dictionary.Type{}).Where("code IN ?", []string{"user_status", "role_status", "menu_type", "api_method", "data_scope"}), 5, "dict types")
	assertCount(t, db.Model(&apigorm.API{}).Where("method = ? AND path = ?", "GET", "/api/admin/users"), 1, "synced api")
	assertCount(t, db.Model(&authgorm.Permission{}).Where("code = ?", "admin.users.get"), 1, "synced api permission")
	assertCount(t, db.Model(&navigation.MenuModel{}).Where("path = ?", "/system/users"), 1, "default menu")
	assertCount(t, db.Model(&identity.User{}).Where("username = ?", conf.Admin.Username), 1, "seed admin")

	adminRole := findRoleByCode(t, db, "admin")
	adminUser := findUserByUsername(t, db, conf.Admin.Username)
	assertCount(t, db.Model(&authgorm.UserRole{}).Where("user_id = ? AND role_id = ?", adminUser.ID, adminRole.ID), 1, "admin user role")
	assertCount(t, db.Model(&authgorm.RolePermission{}).Where("role_id = ?", adminRole.ID), countRows(t, db.Model(&authgorm.Permission{})), "admin role permissions")
	assertCount(t, db.Model(&navigation.RoleMenuModel{}).Where("role_id = ?", adminRole.ID), countRows(t, db.Model(&navigation.MenuModel{})), "admin role menus")
}

func TestRunIsIdempotent(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}
	before := seedTableCounts(t, db)

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("second seed run failed: %v", err)
	}
	after := seedTableCounts(t, db)

	if before != after {
		t.Fatalf("seed is not idempotent\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func TestRunDoesNotMaintainUserAccessVersions(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("initial seed run failed: %v", err)
	}
	adminUser := findUserByUsername(t, db, conf.Admin.Username)
	assertCount(
		t,
		db.Model(&authgorm.UserAccessVersion{}).
			Where("user_id = ?", adminUser.ID),
		0,
		"seed admin access versions",
	)

	if err := db.Model(&adminUser).Update("status", 0).Error; err != nil {
		t.Fatalf("prepare disabled seed admin: %v", err)
	}
	if err := db.Create(&authgorm.UserAccessVersion{
		UserID:  adminUser.ID,
		Version: 7,
	}).Error; err != nil {
		t.Fatalf("create seed admin access version sentinel: %v", err)
	}
	if err := db.Delete(&adminUser).Error; err != nil {
		t.Fatalf("soft delete seed admin: %v", err)
	}

	unrelatedUser := identity.User{
		Username: "seed-unrelated-user",
		Password: "not-used",
		Email:    "seed-unrelated-user@test.local",
		Status:   1,
	}
	if err := db.Create(&unrelatedUser).Error; err != nil {
		t.Fatalf("create unrelated user: %v", err)
	}
	if err := db.Create(&authgorm.UserAccessVersion{
		UserID:  unrelatedUser.ID,
		Version: 13,
	}).Error; err != nil {
		t.Fatalf("create unrelated access version sentinel: %v", err)
	}

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("seed restore run failed: %v", err)
	}

	var restoredAdmin identity.User
	if err := db.First(&restoredAdmin, adminUser.ID).Error; err != nil {
		t.Fatalf("reload restored seed admin: %v", err)
	}
	if restoredAdmin.Status != 1 || restoredAdmin.DeletedAt.Valid {
		t.Fatalf(
			"restored seed admin status/deleted = %d/%v, want 1/false",
			restoredAdmin.Status,
			restoredAdmin.DeletedAt.Valid,
		)
	}
	var adminAccessVersion authgorm.UserAccessVersion
	if err := db.First(
		&adminAccessVersion,
		"user_id = ?",
		adminUser.ID,
	).Error; err != nil {
		t.Fatalf("reload seed admin access version sentinel: %v", err)
	}
	if adminAccessVersion.Version != 7 {
		t.Fatalf(
			"seed changed admin access version = %d, want unchanged 7",
			adminAccessVersion.Version,
		)
	}

	var unrelatedAccessVersion authgorm.UserAccessVersion
	if err := db.First(
		&unrelatedAccessVersion,
		"user_id = ?",
		unrelatedUser.ID,
	).Error; err != nil {
		t.Fatalf("reload unrelated access version sentinel: %v", err)
	}
	if unrelatedAccessVersion.Version != 13 {
		t.Fatalf(
			"seed repaired unrelated access version = %d, want unchanged 13",
			unrelatedAccessVersion.Version,
		)
	}
}

func TestRunRestoresMissingAndSoftDeletedData(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}

	adminRole := findRoleByCode(t, db, "admin")
	var permission authgorm.Permission
	if err := db.Where("code = ?", "admin.users.get").First(&permission).Error; err != nil {
		t.Fatalf("find permission failed: %v", err)
	}
	var menu navigation.MenuModel
	if err := db.Where("path = ?", "/system/users").First(&menu).Error; err != nil {
		t.Fatalf("find menu failed: %v", err)
	}

	if err := db.Where("role_id = ? AND permission_id = ?", adminRole.ID, permission.ID).Delete(&authgorm.RolePermission{}).Error; err != nil {
		t.Fatalf("delete role permission failed: %v", err)
	}
	if err := db.Delete(&menu).Error; err != nil {
		t.Fatalf("soft delete menu failed: %v", err)
	}

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("second seed run failed: %v", err)
	}

	assertCount(t, db.Model(&authgorm.RolePermission{}).Where("role_id = ? AND permission_id = ?", adminRole.ID, permission.ID), 1, "restored admin role permission")
	assertCount(t, db.Model(&navigation.MenuModel{}).Where("path = ?", "/system/users"), 1, "restored soft-deleted menu")
}

func TestRunDoesNotOverwriteEditableMenuFields(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}

	var menu navigation.MenuModel
	if err := db.Where("path = ?", "/system/users").First(&menu).Error; err != nil {
		t.Fatalf("find menu failed: %v", err)
	}
	if err := db.Model(&menu).Updates(map[string]any{
		"name": "Account Center",
		"sort": 99,
	}).Error; err != nil {
		t.Fatalf("update menu failed: %v", err)
	}

	if err := runSeed(db, conf, seedTestRoutes()); err != nil {
		t.Fatalf("second seed run failed: %v", err)
	}

	var updated navigation.MenuModel
	if err := db.Where("path = ?", "/system/users").First(&updated).Error; err != nil {
		t.Fatalf("find updated menu failed: %v", err)
	}
	if updated.Name != "Account Center" || updated.Sort != 99 {
		t.Fatalf("seed overwrote editable menu fields, got name=%q sort=%d", updated.Name, updated.Sort)
	}
}

func setupSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&identity.User{},
		&authgorm.UserAccessVersion{},
		&authgorm.Role{},
		&authgorm.UserRole{},
		&authgorm.PermissionGroup{},
		&authgorm.Permission{},
		&authgorm.RolePermission{},
		&navigation.MenuModel{},
		&navigation.RoleMenuModel{},
		&navigation.MenuAPIModel{},
		&dictionary.Type{},
		&dictionary.Item{},
		&apigorm.API{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	return db
}

func seedTestConfig() platformconfig.Config {
	var conf platformconfig.Config
	conf.Admin.Username = "seed_admin"
	conf.Admin.Password = "SeedAdmin@123456"
	conf.Admin.Email = "seed_admin@test.local"
	conf.Admin.Nickname = "Seed Admin"
	return conf
}

type seedTestRoute struct{ Method, Path string }

func seedTestRoutes() []seedTestRoute {
	return []seedTestRoute{
		{Method: "GET", Path: "/api/admin/users"},
		{Method: "POST", Path: "/api/admin/apis/:id/menu-button"},
		{Method: "POST", Path: "/api/login"},
		{Method: "GET", Path: "/ping"},
	}
}

func runSeed(db *gorm.DB, conf platformconfig.Config, routes []seedTestRoute) error {
	descriptors := make([]routecatalog.Descriptor, len(routes))
	for index, route := range routes {
		access := routecatalog.PermissionControlled
		if route.Path == "/api/login" || route.Path == "/ping" {
			access = routecatalog.Public
		}
		permissionCode := ""
		if access == routecatalog.PermissionControlled {
			permissionCode = application.DerivePermissionCode(route.Method, route.Path)
		}
		descriptors[index] = routecatalog.Descriptor{
			Method: route.Method, Path: route.Path, Access: access,
			Handler: func(*gin.Context) {}, Name: route.Method + " " + route.Path,
			Group: "test", DefaultPermissionCode: permissionCode, DefaultAuditCategory: "test",
			OpenAPI: routecatalog.Operation{Summary: route.Method + " " + route.Path, Request: routecatalog.RequestBody{Kind: routecatalog.NoBody}, Responses: map[int]routecatalog.Response{http.StatusOK: {Description: "ok", Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(map[string]any{})}}},
		}
	}
	catalog, err := routecatalog.New(descriptors)
	if err != nil {
		return err
	}
	return app.SeedCatalog(context.Background(), conf, db, zap.NewNop(), catalog)
}

type seedCounts struct {
	Roles            int64
	PermissionGroups int64
	DictTypes        int64
	DictItems        int64
	APIs             int64
	Permissions      int64
	Menus            int64
	RolePermissions  int64
	RoleMenus        int64
	Users            int64
	UserRoles        int64
}

func seedTableCounts(t *testing.T, db *gorm.DB) seedCounts {
	t.Helper()
	return seedCounts{
		Roles:            countRows(t, db.Model(&authgorm.Role{})),
		PermissionGroups: countRows(t, db.Model(&authgorm.PermissionGroup{})),
		DictTypes:        countRows(t, db.Model(&dictionary.Type{})),
		DictItems:        countRows(t, db.Model(&dictionary.Item{})),
		APIs:             countRows(t, db.Model(&apigorm.API{})),
		Permissions:      countRows(t, db.Model(&authgorm.Permission{})),
		Menus:            countRows(t, db.Model(&navigation.MenuModel{})),
		RolePermissions:  countRows(t, db.Model(&authgorm.RolePermission{})),
		RoleMenus:        countRows(t, db.Model(&navigation.RoleMenuModel{})),
		Users:            countRows(t, db.Model(&identity.User{})),
		UserRoles:        countRows(t, db.Model(&authgorm.UserRole{})),
	}
}

func assertCount(t *testing.T, query *gorm.DB, want int64, name string) {
	t.Helper()
	if got := countRows(t, query); got != want {
		t.Fatalf("%s count mismatch: got %d, want %d", name, got, want)
	}
}

func countRows(t *testing.T, query *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count rows failed: %v", err)
	}
	return count
}

func findRoleByCode(t *testing.T, db *gorm.DB, code string) authgorm.Role {
	t.Helper()
	var role authgorm.Role
	if err := db.Where("code = ?", code).First(&role).Error; err != nil {
		t.Fatalf("find role %q failed: %v", code, err)
	}
	return role
}

func findUserByUsername(t *testing.T, db *gorm.DB, username string) identity.User {
	t.Helper()
	var user identity.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatalf("find user %q failed: %v", username, err)
	}
	return user
}
