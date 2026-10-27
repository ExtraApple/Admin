package application_test

import (
	"context"
	"errors"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
)

// lifecycleAccess keeps authorization persistence real while allowing a precise
// interleaving at the shared identity/role-lock boundary. SQLite does not model
// MySQL row-lock waits; these tests exercise current rechecks and atomicity.
type lifecycleAccess struct {
	db         *gorm.DB
	beforeLock func(context.Context) error
	versionErr error
}

func (access *lifecycleAccess) IsProtectedUser(ctx context.Context, _, targetID uint) (bool, error) {
	if access.beforeLock != nil {
		if err := access.beforeLock(ctx); err != nil {
			return false, err
		}
	}
	var count int64
	err := platformdatabase.FromContext(ctx, access.db).Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ?", targetID, "admin").Count(&count).Error
	return count != 0, err
}

func (access *lifecycleAccess) UserScope(context.Context, uint) (domain.UserScope, error) {
	return domain.UserScope{All: true}, nil
}

func (access *lifecycleAccess) ManagementUserScope(ctx context.Context, userID uint) (domain.UserScope, error) {
	return access.UserScope(ctx, userID)
}

func (access *lifecycleAccess) IncrementVersions(ctx context.Context, ids []uint) error {
	if err := authgorm.NewAccessVersions(access.db).Increment(ctx, ids); err != nil {
		return err
	}
	return access.versionErr
}

func lifecycleFixture(t *testing.T) (*gorm.DB, *identitygorm.Repository, *lifecycleAccess, *application.UserService, domain.User) {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&identitygorm.UserModel{}, &authgorm.Role{}, &authgorm.UserRole{}, &authgorm.UserAccessVersion{}); err != nil {
		t.Fatal(err)
	}
	repository := identitygorm.NewRepository(db)
	user := domain.User{Username: "managed", Email: "managed@example.com", Password: "unchanged-hash", Nickname: "Before", Role: "user", Status: 1}
	if err := repository.Create(context.Background(), &user); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authgorm.UserAccessVersion{UserID: user.ID, Version: 7}).Error; err != nil {
		t.Fatal(err)
	}
	access := &lifecycleAccess{db: db}
	service := application.NewUserService(repository, passwordFake{}, platformdatabase.NewTransactionRunner(db), access)
	return db, repository, access, service, user
}

var lifecycleMutations = []struct {
	name string
	run  func(*application.UserService, uint) error
}{
	{name: "identity status and nickname update", run: func(service *application.UserService, id uint) error {
		status := 0
		_, err := service.UpdateByAdmin(context.Background(), 1000, id, application.AdminUpdateUserRequest{Nickname: "After", Status: &status})
		return err
	}},
	{name: "delete", run: func(service *application.UserService, id uint) error {
		return service.Delete(context.Background(), 1000, id)
	}},
	{name: "toggle", run: func(service *application.UserService, id uint) error {
		_, err := service.ToggleStatus(context.Background(), 1000, id)
		return err
	}},
	{name: "kick", run: func(service *application.UserService, id uint) error {
		return service.Kick(context.Background(), 1000, id)
	}},
}

func TestManagedMutationsRejectUserDeletedAtLockBoundary(t *testing.T) {
	for _, mutation := range lifecycleMutations {
		t.Run(mutation.name, func(t *testing.T) {
			db, repository, access, service, user := lifecycleFixture(t)
			access.beforeLock = func(ctx context.Context) error {
				return platformdatabase.FromContext(ctx, db).Delete(&identitygorm.UserModel{}, user.ID).Error
			}
			err := mutation.run(service, user.ID)
			if code, _ := application.CodeOf(err); code != application.CodeUserNotFound {
				t.Fatalf("mutation error = %v, want user not found after current lock read", err)
			}
			// The interleaved deletion and any mutation/version effects must all
			// roll back because they participate in the owning transaction.
			after, err := repository.FindByID(context.Background(), user.ID)
			if err != nil || after.Nickname != "Before" || after.Status != 1 {
				t.Fatalf("rolled-back user = %#v, %v", after, err)
			}
			version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
			if err != nil || version != 7 {
				t.Fatalf("rolled-back version = %d, %v", version, err)
			}
		})
	}
}

func TestManagedMutationsRollBackIdentityAndVersionTogether(t *testing.T) {
	for _, mutation := range lifecycleMutations {
		t.Run(mutation.name, func(t *testing.T) {
			db, repository, access, service, user := lifecycleFixture(t)
			failure := errors.New("version write rejected after increment")
			access.versionErr = failure
			if err := mutation.run(service, user.ID); !errors.Is(err, failure) {
				t.Fatalf("mutation error = %v, want version failure", err)
			}
			after, err := repository.FindByID(context.Background(), user.ID)
			if err != nil || after.Nickname != "Before" || after.Status != 1 || after.Password != user.Password {
				t.Fatalf("rolled-back identity = %#v, %v", after, err)
			}
			version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
			if err != nil || version != 7 {
				t.Fatalf("rolled-back version = %d, %v", version, err)
			}
		})
	}
}

func TestManagedMutationsRejectCurrentProtectedMembership(t *testing.T) {
	for _, mutation := range lifecycleMutations {
		t.Run(mutation.name, func(t *testing.T) {
			db, repository, access, service, user := lifecycleFixture(t)
			role := authgorm.Role{Name: "Administrator", Code: "admin", Status: 1}
			if err := db.Create(&role).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&role).UpdateColumn("status", 0).Error; err != nil {
				t.Fatal(err)
			}
			access.beforeLock = func(ctx context.Context) error {
				return platformdatabase.FromContext(ctx, db).Create(&authgorm.UserRole{UserID: user.ID, RoleID: role.ID}).Error
			}
			err := mutation.run(service, user.ID)
			if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
				t.Fatalf("mutation error = %v, want current protected-membership rejection", err)
			}
			after, err := repository.FindByID(context.Background(), user.ID)
			if err != nil || after.Nickname != "Before" || after.Status != 1 {
				t.Fatalf("protected identity = %#v, %v", after, err)
			}
			var links int64
			if err := db.Model(&authgorm.UserRole{}).Where("user_id = ?", user.ID).Count(&links).Error; err != nil || links != 0 {
				t.Fatalf("rolled-back protected links = %d, %v", links, err)
			}
		})
	}
}

func TestToggleStatusUsesCurrentLockedStatus(t *testing.T) {
	db, repository, access, service, user := lifecycleFixture(t)
	access.beforeLock = func(ctx context.Context) error {
		return platformdatabase.FromContext(ctx, db).Model(&identitygorm.UserModel{}).Where("id = ?", user.ID).UpdateColumn("status", 0).Error
	}
	status, err := service.ToggleStatus(context.Background(), 1000, user.ID)
	if err != nil || status != 1 {
		t.Fatalf("ToggleStatus() = %d, %v, want inversion of current disabled state", status, err)
	}
	after, err := repository.FindByID(context.Background(), user.ID)
	if err != nil || after.Status != 1 {
		t.Fatalf("persisted status = %d, %v", after.Status, err)
	}
	version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
	if err != nil || version != 8 {
		t.Fatalf("committed version = %d, %v", version, err)
	}
}

func TestDeleteRetainsManagementFactsAndVersionButHidesPublicDirectory(t *testing.T) {
	db, repository, _, service, user := lifecycleFixture(t)
	if err := service.Delete(context.Background(), 1000, user.ID); err != nil {
		t.Fatal(err)
	}
	public, err := repository.ListUsersByIDs(context.Background(), []uint{user.ID})
	if err != nil || len(public) != 0 {
		t.Fatalf("public directory includes deleted user: %#v, %v", public, err)
	}
	ids, err := repository.ListUserIDs(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatalf("public user IDs include deleted user: %v, %v", ids, err)
	}
	facts, err := repository.ManagementUsersByIDs(context.Background(), []uint{user.ID})
	if err != nil || len(facts) != 1 || facts[0].ID != user.ID || !facts[0].Deleted {
		t.Fatalf("retained management facts = %#v, %v", facts, err)
	}
	version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
	if err != nil || version != 8 {
		t.Fatalf("retained version = %d, %v", version, err)
	}
}

func TestPasswordAndAuthorizationVersionCommitAtomically(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "commit"
		if fail {
			name = "rollback-version-failure"
		}
		t.Run(name, func(t *testing.T) {
			db, repository, access, service, user := lifecycleFixture(t)
			if fail {
				access.versionErr = errors.New("version write failed")
			}
			err := service.ChangePassword(context.Background(), user.ID, application.ChangePasswordRequest{OldPassword: "OldPass123!", NewPassword: "NewPass123!", ConfirmPassword: "NewPass123!"})
			if fail {
				if code, _ := application.CodeOf(err); code != application.CodeInternalError {
					t.Fatalf("password failure = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			persisted, err := repository.FindByID(context.Background(), user.ID)
			if err != nil {
				t.Fatal(err)
			}
			version, err := authgorm.NewAccessVersions(db).Current(context.Background(), user.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantPassword, wantVersion := "hash", 8
			if fail {
				wantPassword, wantVersion = user.Password, 7
			}
			if persisted.Password != wantPassword || version != wantVersion {
				t.Fatalf("password/version = %q/%d, want atomic %q/%d", persisted.Password, version, wantPassword, wantVersion)
			}
		})
	}
}
