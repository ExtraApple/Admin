package initialize

import (
	"testing"

	"go.uber.org/zap"

	"admin/global"
	"admin/initialize/testutil"
	"admin/model"
)

func TestInitSuperAdminDoesNotMaintainUserAccessVersions(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
		&model.Role{},
		&model.UserRole{},
	); err != nil {
		t.Fatalf("migrate super-administrator test tables: %v", err)
	}

	previousDB := global.DB
	previousLogger := global.Logger
	global.DB = db
	global.Logger = zap.NewNop()
	t.Cleanup(func() {
		global.DB = previousDB
		global.Logger = previousLogger
	})

	conf := Config{}
	conf.Admin.Username = "initialize-admin"
	conf.Admin.Password = "InitializeAdmin@123456"
	conf.Admin.Email = "initialize-admin@test.local"
	conf.Admin.Nickname = "Initialize Admin"

	InitSuperAdmin(&conf)

	var adminUser model.User
	if err := db.Where("username = ?", conf.Admin.Username).
		First(&adminUser).Error; err != nil {
		t.Fatalf("load initialized super administrator: %v", err)
	}
	var accessVersionCount int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", adminUser.ID).
		Count(&accessVersionCount).Error; err != nil {
		t.Fatalf("count initialized administrator access versions: %v", err)
	}
	if accessVersionCount != 0 {
		t.Fatalf(
			"initialized administrator access-version rows = %d, want 0",
			accessVersionCount,
		)
	}

	if err := db.Model(&adminUser).Update("status", 0).Error; err != nil {
		t.Fatalf("prepare disabled administrator: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  adminUser.ID,
		Version: 7,
	}).Error; err != nil {
		t.Fatalf("create administrator access-version sentinel: %v", err)
	}
	if err := db.Delete(&adminUser).Error; err != nil {
		t.Fatalf("soft delete administrator: %v", err)
	}

	InitSuperAdmin(&conf)

	var restored model.User
	if err := db.First(&restored, adminUser.ID).Error; err != nil {
		t.Fatalf("reload restored administrator: %v", err)
	}
	if restored.Status != 1 || restored.DeletedAt.Valid {
		t.Fatalf(
			"restored administrator status/deleted = %d/%v, want 1/false",
			restored.Status,
			restored.DeletedAt.Valid,
		)
	}
	var accessVersion model.UserAccessVersion
	if err := db.First(
		&accessVersion,
		"user_id = ?",
		adminUser.ID,
	).Error; err != nil {
		t.Fatalf("reload administrator access-version sentinel: %v", err)
	}
	if accessVersion.Version != 7 {
		t.Fatalf(
			"initializer changed access version = %d, want unchanged 7",
			accessVersion.Version,
		)
	}
}
