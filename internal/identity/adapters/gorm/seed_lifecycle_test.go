package gormadapter_test

import (
	"context"
	"errors"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	identitygorm "admin/internal/identity/adapters/gorm"
	platformconfig "admin/internal/platform/config"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
)

func seedLifecycleFixture(t *testing.T) (*gorm.DB, *platformconfig.Config, identitygorm.UserModel, authgorm.Role) {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&identitygorm.UserModel{}, &authgorm.Role{}, &authgorm.UserRole{}, &authgorm.UserAccessVersion{}); err != nil {
		t.Fatal(err)
	}
	conf := &platformconfig.Config{}
	conf.Admin.Username = "seed-lifecycle"
	conf.Admin.Password = "ReplacementPass123!"
	conf.Admin.Email = "seed-lifecycle@example.com"
	role := authgorm.Role{Name: "Administrator", Code: "admin", Status: 1}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	user := identitygorm.UserModel{Username: conf.Admin.Username, Email: conf.Admin.Email, Password: "original-password-hash", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authgorm.UserRole{UserID: user.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authgorm.UserAccessVersion{UserID: user.ID, Version: 7}).Error; err != nil {
		t.Fatal(err)
	}
	return db, conf, user, role
}

func TestSeedAdminRestorationRollsBackWithOwningTransaction(t *testing.T) {
	db, conf, user, _ := seedLifecycleFixture(t)
	if err := db.Model(&user).UpdateColumn("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("later seed owner failed")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := identitygorm.SeedAdminUser(tx, conf); err != nil {
			return err
		}
		var restored identitygorm.UserModel
		if err := tx.First(&restored, user.ID).Error; err != nil {
			return err
		}
		if restored.Status != 1 || restored.DeletedAt.Valid {
			t.Fatalf("user not restored inside owning transaction: %#v", restored)
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("seed transaction error = %v", err)
	}
	var after identitygorm.UserModel
	if err := db.Unscoped().First(&after, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != 0 || !after.DeletedAt.Valid || after.Password != user.Password {
		t.Fatalf("seed restoration escaped rollback: %#v", after)
	}
	version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
	if err != nil || version != 7 {
		t.Fatalf("retained authorization version = %d, %v", version, err)
	}
}

func TestSeedAdminDoesNotRecreateClearedBindingOrRewritePassword(t *testing.T) {
	db, conf, user, role := seedLifecycleFixture(t)
	if err := db.Model(&user).UpdateColumn("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&authgorm.UserRole{}, "user_id = ? AND role_id = ?", user.ID, role.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return identitygorm.SeedAdminUser(tx, conf) }); err == nil {
		t.Fatal("seed accepted a retained user whose administrator binding was cleared")
	}
	var after identitygorm.UserModel
	if err := db.Unscoped().First(&after, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != 0 || !after.DeletedAt.Valid || after.Password != user.Password {
		t.Fatalf("rejected restoration changed identity: %#v", after)
	}
	var links int64
	if err := db.Model(&authgorm.UserRole{}).Where("user_id = ?", user.ID).Count(&links).Error; err != nil || links != 0 {
		t.Fatalf("cleared bindings were recreated: count=%d err=%v", links, err)
	}
	version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
	if err != nil || version != 7 {
		t.Fatalf("retained authorization version = %d, %v", version, err)
	}
}

func TestSeedAdminCountErrorDoesNotRewriteActiveAdministrator(t *testing.T) {
	db, conf, user, _ := seedLifecycleFixture(t)
	failure := errors.New("administrator count failed")
	const callback = "test:seed-lifecycle-count-failure"
	if err := db.Callback().Query().Before("gorm:query").Register(callback, func(query *gorm.DB) {
		if query.Statement.Table == "users" {
			query.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := db.Transaction(func(tx *gorm.DB) error { return identitygorm.SeedAdminUser(tx, conf) })
	if removeErr := db.Callback().Query().Remove(callback); removeErr != nil {
		t.Fatal(removeErr)
	}
	if !errors.Is(err, failure) {
		t.Fatalf("seed error = %v, want count failure", err)
	}
	var after identitygorm.UserModel
	if err := db.First(&after, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Password != user.Password || after.Status != 1 {
		t.Fatalf("count failure rewrote active administrator: %#v", after)
	}
}

func TestSeedAdminPreservesActiveAdministratorPassword(t *testing.T) {
	db, conf, user, _ := seedLifecycleFixture(t)
	if err := db.Transaction(func(tx *gorm.DB) error { return identitygorm.SeedAdminUser(tx, conf) }); err != nil {
		t.Fatal(err)
	}
	var after identitygorm.UserModel
	if err := db.First(&after, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Password != user.Password || after.Status != 1 {
		t.Fatalf("active administrator rewritten by seed: %#v", after)
	}
	version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
	if err != nil || version != 7 {
		t.Fatalf("active administrator version = %d, %v", version, err)
	}
}
