package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/identity/application"
	platformdatabase "admin/internal/platform/database"
)

func TestAdminNicknameAndSameFactsPreserveExistingOrAbsentVersions(t *testing.T) {
	for _, hasVersion := range []bool{true, false} {
		name := "absent version"
		if hasVersion {
			name = "existing version"
		}
		t.Run(name, func(t *testing.T) {
			db, repository, access, service, user := lifecycleFixture(t)
			ctx := context.Background()
			if !hasVersion {
				if err := db.Delete(&authgorm.UserAccessVersion{}, "user_id = ?", user.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			// Advancing a version would perform a real owner write and then fail.
			// Pure profile changes and accepted identical facts must not do so.
			access.versionErr = errors.New("unexpected authorization mutation")
			status := user.Status
			for _, request := range []application.AdminUpdateUserRequest{
				{Nickname: "  After  "},
				{Nickname: "  After  ", Role: user.Role, Status: &status},
			} {
				result, err := service.UpdateByAdmin(ctx, 1000, user.ID, request)
				if err != nil || result.Nickname != "  After  " || result.Role != user.Role || result.Status != user.Status {
					t.Fatalf("profile result=%#v error=%v", result, err)
				}
				stored, err := repository.FindByID(ctx, user.ID)
				if err != nil || stored.Nickname != "  After  " || stored.Email != user.Email || stored.Password != user.Password {
					t.Fatalf("stored profile=%#v error=%v", stored, err)
				}
				version, err := authgorm.NewAccessVersions(db).Current(ctx, user.ID)
				if hasVersion {
					if err != nil || version != 7 {
						t.Fatalf("profile advanced version=%d error=%v", version, err)
					}
				} else if !errors.Is(err, authgorm.ErrAccessVersionNotFound) {
					t.Fatalf("profile initialized absent version=%d error=%v", version, err)
				}
			}
		})
	}
}

func TestAdminMixedNicknameAndIdentityChangesAdvanceVersionOnce(t *testing.T) {
	for _, field := range []string{"status", "legacy role", "both"} {
		t.Run(field, func(t *testing.T) {
			db, repository, _, service, user := lifecycleFixture(t)
			ctx := context.Background()
			status := 0
			request := application.AdminUpdateUserRequest{Nickname: "After"}
			if field != "legacy role" {
				request.Status = &status
			}
			if field != "status" {
				request.Role = "legacy-editor"
			}
			result, err := service.UpdateByAdmin(ctx, 1000, user.ID, request)
			if err != nil || result.Nickname != "After" {
				t.Fatalf("mixed update=%#v error=%v", result, err)
			}
			stored, err := repository.FindByID(ctx, user.ID)
			if err != nil || (request.Status != nil && stored.Status != 0) || (request.Role != "" && stored.Role != request.Role) {
				t.Fatalf("stored mixed update=%#v error=%v", stored, err)
			}
			version, err := authgorm.NewAccessVersions(db).Current(ctx, user.ID)
			if err != nil || version != 8 {
				t.Fatalf("mixed update version=%d error=%v want=8", version, err)
			}
			var links int64
			if err := db.Table("user_roles").Where("user_id = ?", user.ID).Count(&links).Error; err != nil || links != 0 {
				t.Fatalf("legacy field invented RBAC identity: links=%d error=%v", links, err)
			}
		})
	}
}

func TestAdminIdentityComparisonUsesCurrentLockedFacts(t *testing.T) {
	db, repository, access, service, user := lifecycleFixture(t)
	ctx := context.Background()
	access.beforeLock = func(tx context.Context) error {
		if err := platformdatabase.FromContext(tx, db).Model(&identitygorm.UserModel{}).Where("id = ?", user.ID).UpdateColumn("status", 0).Error; err != nil {
			return err
		}
		return authgorm.NewAccessVersions(db).Increment(tx, []uint{user.ID})
	}
	oldSubmittedStatus := 1
	result, err := service.UpdateByAdmin(ctx, 1000, user.ID, application.AdminUpdateUserRequest{Nickname: "After", Status: &oldSubmittedStatus})
	if err != nil || result.Status != 1 {
		t.Fatalf("current-fact result=%#v error=%v", result, err)
	}
	stored, err := repository.FindByID(ctx, user.ID)
	if err != nil || stored.Status != 1 || stored.Nickname != "After" {
		t.Fatalf("current-fact storage=%#v error=%v", stored, err)
	}
	version, err := authgorm.NewAccessVersions(db).Current(ctx, user.ID)
	if err != nil || version != 9 {
		t.Fatalf("status changed since pre-read but version=%d error=%v want=9", version, err)
	}
}

func TestAdminProfileWriteAndNewVersionFailuresRollback(t *testing.T) {
	for _, versionFailure := range []bool{false, true} {
		name := "profile storage failure"
		if versionFailure {
			name = "missing version initialization then failure"
		}
		t.Run(name, func(t *testing.T) {
			db, repository, access, service, user := lifecycleFixture(t)
			ctx := context.Background()
			request := application.AdminUpdateUserRequest{Nickname: "After"}
			if versionFailure {
				if err := db.Delete(&authgorm.UserAccessVersion{}, "user_id = ?", user.ID).Error; err != nil {
					t.Fatal(err)
				}
				status := 0
				request.Status = &status
				access.versionErr = errors.New("failure after initializing and advancing version")
			} else if err := db.Exec("CREATE TRIGGER nickname_storage_failure BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT, 'nickname_storage_failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			before, err := repository.FindByID(ctx, user.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.UpdateByAdmin(ctx, 1000, user.ID, request); err == nil {
				t.Fatal("failing owner write succeeded")
			}
			after, err := repository.FindByID(ctx, user.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("failed update retained profile changes: before=%#v after=%#v error=%v", before, after, err)
			}
			version, err := authgorm.NewAccessVersions(db).Current(ctx, user.ID)
			if versionFailure {
				if !errors.Is(err, authgorm.ErrAccessVersionNotFound) {
					t.Fatalf("failed update retained initialized version=%d error=%v", version, err)
				}
			} else if err != nil || version != 7 {
				t.Fatalf("profile failure changed version=%d error=%v", version, err)
			}
		})
	}
}
