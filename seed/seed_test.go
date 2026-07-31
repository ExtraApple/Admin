package seed

import (
	"testing"

	"admin/dto"
	"admin/global"
	"admin/initialize"
	"admin/model"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRunInitializesRequiredData(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("run seed failed: %v", err)
	}

	assertCount(t, db.Model(&model.Role{}).Where("code IN ?", []string{"admin", "user"}), 2, "default roles")
	assertCount(t, db.Model(&model.PermissionGroup{}).Where("name IN ?", []string{"auth", "user", "role", "permission", "menu", "api", "organization", "dict", "file", "audit", "system"}), 11, "permission groups")
	assertCount(t, db.Model(&model.DictType{}).Where("code IN ?", []string{"user_status", "role_status", "menu_type", "api_method", "data_scope"}), 5, "dict types")
	assertCount(t, db.Model(&model.API{}).Where("method = ? AND path = ?", "GET", "/api/admin/users"), 1, "synced api")
	assertCount(t, db.Model(&model.Permission{}).Where("code = ?", "admin.users.get"), 1, "synced api permission")
	assertCount(t, db.Model(&model.Menu{}).Where("path = ?", "/system/users"), 1, "default menu")
	assertCount(t, db.Model(&model.User{}).Where("username = ?", conf.Admin.Username), 1, "seed admin")

	adminRole := findRoleByCode(t, db, "admin")
	adminUser := findUserByUsername(t, db, conf.Admin.Username)
	assertCount(t, db.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", adminUser.ID, adminRole.ID), 1, "admin user role")
	assertCount(t, db.Model(&model.RolePermission{}).Where("role_id = ?", adminRole.ID), countRows(t, db.Model(&model.Permission{})), "admin role permissions")
	assertCount(t, db.Model(&model.RoleMenu{}).Where("role_id = ?", adminRole.ID), countRows(t, db.Model(&model.Menu{})), "admin role menus")
}

func TestRunIsIdempotent(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}
	before := seedTableCounts(t, db)

	if err := Run(&conf, seedTestRoutes()); err != nil {
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

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("initial seed run failed: %v", err)
	}
	adminUser := findUserByUsername(t, db, conf.Admin.Username)
	assertCount(
		t,
		db.Model(&model.UserAccessVersion{}).
			Where("user_id = ?", adminUser.ID),
		0,
		"seed admin access versions",
	)

	if err := db.Model(&adminUser).Update("status", 0).Error; err != nil {
		t.Fatalf("prepare disabled seed admin: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  adminUser.ID,
		Version: 7,
	}).Error; err != nil {
		t.Fatalf("create seed admin access version sentinel: %v", err)
	}
	if err := db.Delete(&adminUser).Error; err != nil {
		t.Fatalf("soft delete seed admin: %v", err)
	}

	unrelatedUser := model.User{
		Username: "seed-unrelated-user",
		Password: "not-used",
		Email:    "seed-unrelated-user@test.local",
		Status:   1,
	}
	if err := db.Create(&unrelatedUser).Error; err != nil {
		t.Fatalf("create unrelated user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  unrelatedUser.ID,
		Version: 13,
	}).Error; err != nil {
		t.Fatalf("create unrelated access version sentinel: %v", err)
	}

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("seed restore run failed: %v", err)
	}

	var restoredAdmin model.User
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
	var adminAccessVersion model.UserAccessVersion
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

	var unrelatedAccessVersion model.UserAccessVersion
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

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}

	adminRole := findRoleByCode(t, db, "admin")
	var permission model.Permission
	if err := db.Where("code = ?", "admin.users.get").First(&permission).Error; err != nil {
		t.Fatalf("find permission failed: %v", err)
	}
	var menu model.Menu
	if err := db.Where("path = ?", "/system/users").First(&menu).Error; err != nil {
		t.Fatalf("find menu failed: %v", err)
	}

	if err := db.Where("role_id = ? AND permission_id = ?", adminRole.ID, permission.ID).Delete(&model.RolePermission{}).Error; err != nil {
		t.Fatalf("delete role permission failed: %v", err)
	}
	if err := db.Delete(&menu).Error; err != nil {
		t.Fatalf("soft delete menu failed: %v", err)
	}

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("second seed run failed: %v", err)
	}

	assertCount(t, db.Model(&model.RolePermission{}).Where("role_id = ? AND permission_id = ?", adminRole.ID, permission.ID), 1, "restored admin role permission")
	assertCount(t, db.Model(&model.Menu{}).Where("path = ?", "/system/users"), 1, "restored soft-deleted menu")
}

func TestRunDoesNotOverwriteEditableMenuFields(t *testing.T) {
	db := setupSeedTestDB(t)
	conf := seedTestConfig()

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}

	var menu model.Menu
	if err := db.Where("path = ?", "/system/users").First(&menu).Error; err != nil {
		t.Fatalf("find menu failed: %v", err)
	}
	if err := db.Model(&menu).Updates(map[string]any{
		"name": "Account Center",
		"sort": 99,
	}).Error; err != nil {
		t.Fatalf("update menu failed: %v", err)
	}

	if err := Run(&conf, seedTestRoutes()); err != nil {
		t.Fatalf("second seed run failed: %v", err)
	}

	var updated model.Menu
	if err := db.Where("path = ?", "/system/users").First(&updated).Error; err != nil {
		t.Fatalf("find updated menu failed: %v", err)
	}
	if updated.Name != "Account Center" || updated.Sort != 99 {
		t.Fatalf("seed overwrote editable menu fields, got name=%q sort=%d", updated.Name, updated.Sort)
	}
}

func setupSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := global.DB
	previousLogger := global.Logger
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(model.Models...); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	global.DB = db
	global.Logger = zap.NewNop()
	return db
}

func seedTestConfig() initialize.Config {
	var conf initialize.Config
	conf.Admin.Username = "seed_admin"
	conf.Admin.Password = "SeedAdmin@123456"
	conf.Admin.Email = "seed_admin@test.local"
	conf.Admin.Nickname = "Seed Admin"
	return conf
}

func seedTestRoutes() []dto.SyncAPIItem {
	return []dto.SyncAPIItem{
		{Method: "GET", Path: "/api/admin/users"},
		{Method: "POST", Path: "/api/admin/apis/:id/menu-button"},
		{Method: "POST", Path: "/api/login"},
		{Method: "GET", Path: "/ping"},
	}
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
		Roles:            countRows(t, db.Model(&model.Role{})),
		PermissionGroups: countRows(t, db.Model(&model.PermissionGroup{})),
		DictTypes:        countRows(t, db.Model(&model.DictType{})),
		DictItems:        countRows(t, db.Model(&model.DictItem{})),
		APIs:             countRows(t, db.Model(&model.API{})),
		Permissions:      countRows(t, db.Model(&model.Permission{})),
		Menus:            countRows(t, db.Model(&model.Menu{})),
		RolePermissions:  countRows(t, db.Model(&model.RolePermission{})),
		RoleMenus:        countRows(t, db.Model(&model.RoleMenu{})),
		Users:            countRows(t, db.Model(&model.User{})),
		UserRoles:        countRows(t, db.Model(&model.UserRole{})),
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

func findRoleByCode(t *testing.T, db *gorm.DB, code string) model.Role {
	t.Helper()
	var role model.Role
	if err := db.Where("code = ?", code).First(&role).Error; err != nil {
		t.Fatalf("find role %q failed: %v", code, err)
	}
	return role
}

func findUserByUsername(t *testing.T, db *gorm.DB, username string) model.User {
	t.Helper()
	var user model.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatalf("find user %q failed: %v", username, err)
	}
	return user
}
