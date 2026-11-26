package application_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	authgorm "admin/internal/authorization/adapters/gorm"
	identitymodule "admin/internal/identity"
	identitygorm "admin/internal/identity/adapters/gorm"
	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/organization"
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

func (access *lifecycleAccess) EnsureVersions(ctx context.Context, ids []uint) (map[uint]int, error) {
	return authgorm.NewAccessVersions(access.db).EnsureMany(ctx, ids)
}

func lifecycleFixture(t *testing.T) (*gorm.DB, *identitygorm.Repository, *lifecycleAccess, *application.UserService, domain.User) {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(&identitygorm.UserModel{}, &identitymodule.EmailVerificationCredential{}, &authgorm.Role{}, &authgorm.UserRole{}, &authgorm.UserAccessVersion{}); err != nil {
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

func TestDeleteClearsPendingEmailAndInvalidatesAllUnusedCredentialsAtomically(t *testing.T) {
	t.Run("commits pending_email clear and invalidates all unused credentials", func(t *testing.T) {
		db, repository, _, service, user := lifecycleFixture(t)
		ctx := context.Background()

		pendingEmail := "pending@example.com"
		if err := repository.Update(ctx, user.ID, application.UserChanges{PendingEmail: &pendingEmail}); err != nil {
			t.Fatal(err)
		}
		other := domain.User{Username: "other", Email: "other@example.com", Password: "hash", Role: "user", Status: 1}
		if err := repository.Create(ctx, &other); err != nil {
			t.Fatal(err)
		}

		now := time.Now().UTC().Truncate(time.Second)
		earlierUsed := now.Add(-2 * time.Hour)
		pendingRaw := bytes.Repeat([]byte{0x11}, 32)
		currentRaw := bytes.Repeat([]byte{0x22}, 32)
		pendingDigest := sha256.Sum256(pendingRaw)
		currentDigest := sha256.Sum256(currentRaw)

		pendingCred := identitymodule.EmailVerificationCredential{
			UserID:    user.ID,
			Email:     pendingEmail,
			TokenHash: hex.EncodeToString(pendingDigest[:]),
			ExpiresAt: now.Add(15 * time.Minute),
		}
		currentCred := identitymodule.EmailVerificationCredential{
			UserID:    user.ID,
			Email:     user.Email,
			TokenHash: hex.EncodeToString(currentDigest[:]),
			ExpiresAt: now.Add(15 * time.Minute),
		}
		usedCred := identitymodule.EmailVerificationCredential{
			UserID:    user.ID,
			Email:     "historical@example.com",
			TokenHash: hex.EncodeToString(bytes.Repeat([]byte{0x33}, 32)),
			ExpiresAt: now.Add(15 * time.Minute),
			UsedAt:    &earlierUsed,
		}
		otherCred := identitymodule.EmailVerificationCredential{
			UserID:    other.ID,
			Email:     other.Email,
			TokenHash: hex.EncodeToString(bytes.Repeat([]byte{0x44}, 32)),
			ExpiresAt: now.Add(15 * time.Minute),
		}
		for _, cred := range []*identitymodule.EmailVerificationCredential{&pendingCred, &currentCred, &usedCred, &otherCred} {
			if err := db.Create(cred).Error; err != nil {
				t.Fatal(err)
			}
		}

		if err := service.Delete(ctx, 1000, user.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		var stored identitygorm.UserModel
		if err := db.Unscoped().First(&stored, user.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !stored.DeletedAt.Valid {
			t.Fatal("deleted user missing deleted_at")
		}
		if stored.PendingEmail != "" {
			t.Fatalf("deleted user pending_email = %q, want empty", stored.PendingEmail)
		}
		if stored.Email != user.Email {
			t.Fatalf("deleted user email = %q, want preserved %q", stored.Email, user.Email)
		}

		for _, tc := range []struct {
			name string
			id   uint
		}{
			{"pending email credential", pendingCred.ID},
			{"current unverified email credential", currentCred.ID},
		} {
			var loaded identitymodule.EmailVerificationCredential
			if err := db.First(&loaded, tc.id).Error; err != nil {
				t.Fatal(err)
			}
			if loaded.UsedAt == nil {
				t.Fatalf("%s used_at = nil, want invalidated timestamp", tc.name)
			}
		}

		var loadedUsed identitymodule.EmailVerificationCredential
		if err := db.First(&loadedUsed, usedCred.ID).Error; err != nil {
			t.Fatal(err)
		}
		if loadedUsed.UsedAt == nil || !loadedUsed.UsedAt.UTC().Equal(earlierUsed) {
			t.Fatalf("already-used credential used_at = %v, want preserved %v", loadedUsed.UsedAt, earlierUsed)
		}

		var loadedOther identitymodule.EmailVerificationCredential
		if err := db.First(&loadedOther, otherCred.ID).Error; err != nil {
			t.Fatal(err)
		}
		if loadedOther.UsedAt != nil {
			t.Fatalf("other user's unused credential was mutated: used_at = %v", loadedOther.UsedAt)
		}

		if conflict, err := repository.ExistsIdentityConflict(ctx, "fresh", pendingEmail); err != nil || conflict {
			t.Fatalf("ExistsIdentityConflict(cleared pending_email) = %v, %v; want false", conflict, err)
		}
		if conflict, err := repository.ExistsIdentityConflict(ctx, "fresh", user.Email); err != nil || !conflict {
			t.Fatalf("ExistsIdentityConflict(deleted primary email) = %v, %v; want true", conflict, err)
		}

		// Even if the soft-deleted user is later restored, pre-delete verification tokens cannot be replayed.
		if err := db.Unscoped().Model(&identitygorm.UserModel{}).Where("id = ?", user.ID).Update("deleted_at", nil).Error; err != nil {
			t.Fatal(err)
		}
		verification := application.NewEmailVerificationService(repository, platformdatabase.NewTransactionRunner(db), application.WithEmailVerificationClock(func() time.Time { return now }))
		for _, raw := range [][]byte{pendingRaw, currentRaw} {
			err := verification.Confirm(ctx, user.ID, base64.RawURLEncoding.EncodeToString(raw))
			if code, ok := application.CodeOf(err); !ok || code != application.CodeEmailVerificationInvalid {
				t.Fatalf("Confirm(pre-delete token after restore) err = %v (code %q), want %q", err, code, application.CodeEmailVerificationInvalid)
			}
		}
	})

	t.Run("rolls back pending_email clear and credential invalidation on version failure", func(t *testing.T) {
		db, repository, access, service, user := lifecycleFixture(t)
		ctx := context.Background()

		pendingEmail := "rollback-pending@example.com"
		if err := repository.Update(ctx, user.ID, application.UserChanges{PendingEmail: &pendingEmail}); err != nil {
			t.Fatal(err)
		}
		cred := identitymodule.EmailVerificationCredential{
			UserID:    user.ID,
			Email:     pendingEmail,
			TokenHash: hex.EncodeToString(bytes.Repeat([]byte{0x55}, 32)),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		}
		if err := db.Create(&cred).Error; err != nil {
			t.Fatal(err)
		}

		failure := errors.New("version write failed")
		access.versionErr = failure
		if err := service.Delete(ctx, 1000, user.ID); !errors.Is(err, failure) {
			t.Fatalf("Delete() error = %v, want %v", err, failure)
		}

		after, err := repository.FindByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("FindByID after rollback error = %v", err)
		}
		if after.PendingEmail != pendingEmail {
			t.Fatalf("rolled-back pending_email = %q, want %q", after.PendingEmail, pendingEmail)
		}
		var loaded identitymodule.EmailVerificationCredential
		if err := db.First(&loaded, cred.ID).Error; err != nil {
			t.Fatal(err)
		}
		if loaded.UsedAt != nil {
			t.Fatalf("rolled-back credential used_at = %v, want nil", loaded.UsedAt)
		}
	})
}

type recoveryAccess struct {
	db              *gorm.DB
	scope           domain.UserScope
	managementScope *domain.UserScope
	beforeLock      func(context.Context) error
	versionErr      error
	roleClearErr    error
}

func (access *recoveryAccess) IsProtectedUser(ctx context.Context, _, targetID uint) (bool, error) {
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

func (access *recoveryAccess) UserScope(context.Context, uint) (domain.UserScope, error) {
	return access.scope, nil
}

func (access *recoveryAccess) ManagementUserScope(ctx context.Context, userID uint) (domain.UserScope, error) {
	if access.managementScope != nil {
		return *access.managementScope, nil
	}
	return access.UserScope(ctx, userID)
}

func (access *recoveryAccess) IncrementVersions(ctx context.Context, ids []uint) error {
	if err := authgorm.NewAccessVersions(access.db).Increment(ctx, ids); err != nil {
		return err
	}
	return access.versionErr
}

func (access *recoveryAccess) EnsureVersions(ctx context.Context, ids []uint) (map[uint]int, error) {
	return authgorm.NewAccessVersions(access.db).EnsureMany(ctx, ids)
}

func (access *recoveryAccess) UserRoles(ctx context.Context, ids []uint) ([]application.UserRoleFact, error) {
	if len(ids) == 0 {
		return []application.UserRoleFact{}, nil
	}
	var rows []application.UserRoleFact
	err := platformdatabase.FromContext(ctx, access.db).Table("roles").
		Select("user_roles.user_id AS user_id, roles.id AS id, roles.code AS code, roles.name AS name, roles.status AS status").
		Joins("JOIN user_roles ON user_roles.role_id = roles.id").
		Where("user_roles.user_id IN ?", ids).
		Order("roles.id asc").
		Scan(&rows).Error
	return rows, err
}

func (access *recoveryAccess) ClearUserRoles(ctx context.Context, userID uint) error {
	if access.roleClearErr != nil {
		return access.roleClearErr
	}
	return platformdatabase.FromContext(ctx, access.db).Where("user_id = ?", userID).Delete(&authgorm.UserRole{}).Error
}

type recoveryOrgs struct {
	db       *gorm.DB
	clearErr error
}

func (orgs *recoveryOrgs) UserOrganizations(ctx context.Context, _ uint, ids []uint) ([]application.UserOrganizationFact, error) {
	if len(ids) == 0 {
		return []application.UserOrganizationFact{}, nil
	}
	var rows []struct {
		UserID uint
		ID     uint
		Name   string
	}
	err := platformdatabase.FromContext(ctx, orgs.db).Table("organizations").
		Select("user_organizations.user_id AS user_id, organizations.id AS id, organizations.name AS name").
		Joins("JOIN user_organizations ON user_organizations.organization_id = organizations.id").
		Where("user_organizations.user_id IN ?", ids).
		Order("organizations.id asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	facts := make([]application.UserOrganizationFact, len(rows))
	for index, row := range rows {
		facts[index] = application.UserOrganizationFact{UserID: row.UserID, ID: row.ID, Name: row.Name, Manageable: true}
	}
	return facts, nil
}

func (orgs *recoveryOrgs) ClearUserOrganizations(ctx context.Context, userID uint) error {
	if orgs.clearErr != nil {
		return orgs.clearErr
	}
	return platformdatabase.FromContext(ctx, orgs.db).Where("user_id = ?", userID).Delete(&organization.Membership{}).Error
}

type avatarDeleteCall struct {
	bucket string
	object string
}

type recordingAvatarCleaner struct {
	calls    []avatarDeleteCall
	onDelete func(bucket, object string) error
	err      error
}

func (cleaner *recordingAvatarCleaner) Delete(_ context.Context, bucket, object string) error {
	cleaner.calls = append(cleaner.calls, avatarDeleteCall{bucket: bucket, object: object})
	if cleaner.onDelete != nil {
		if err := cleaner.onDelete(bucket, object); err != nil {
			return err
		}
	}
	return cleaner.err
}

type recordingLifecycleLogger struct {
	failedUserIDs []uint
}

func (logger *recordingLifecycleLogger) WarnAvatarCleanupFailed(userID uint) {
	logger.failedUserIDs = append(logger.failedUserIDs, userID)
}

type recoveryFixtureEnv struct {
	db       *gorm.DB
	repo     *identitygorm.Repository
	access   *recoveryAccess
	orgs     *recoveryOrgs
	avatars  *recordingAvatarCleaner
	logger   *recordingLifecycleLogger
	users    *application.UserService
	admin    *application.AdminUserService
	target   domain.User
}

func newRecoveryFixture(t *testing.T) *recoveryFixtureEnv {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&identitygorm.UserModel{},
		&identitymodule.EmailVerificationCredential{},
		&authgorm.Role{},
		&authgorm.UserRole{},
		&authgorm.UserAccessVersion{},
		&organization.Unit{},
		&organization.Membership{},
	); err != nil {
		t.Fatal(err)
	}
	repo := identitygorm.NewRepository(db)
	tx := platformdatabase.NewTransactionRunner(db)
	verifiedAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	target := domain.User{
		Username:        "recoverable",
		Email:           "recoverable@example.test",
		Password:        "stored-hash",
		Nickname:        "Recoverable User",
		Role:            "user",
		Status:          1,
		EmailVerifiedAt: &verifiedAt,
	}
	if err := repo.Create(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&authgorm.UserAccessVersion{UserID: target.ID, Version: 7}).Error; err != nil {
		t.Fatal(err)
	}
	access := &recoveryAccess{db: db, scope: domain.UserScope{All: true}}
	orgs := &recoveryOrgs{db: db}
	avatars := &recordingAvatarCleaner{}
	logger := &recordingLifecycleLogger{}
	admin := application.NewAdminUserService(repo, access, orgs, tx)
	users := application.NewUserService(repo, passwordFake{}, tx, access)
	users.ConfigureLifecycle(access, orgs, avatars, admin, logger)
	return &recoveryFixtureEnv{db: db, repo: repo, access: access, orgs: orgs, avatars: avatars, logger: logger, users: users, admin: admin, target: target}
}

func TestRestoreKeepsInvalidatedTokensCredentialsAndDoesNotReviveClearedMemberships(t *testing.T) {
	env := newRecoveryFixture(t)
	ctx := context.Background()

	roleKeep := authgorm.Role{Name: "Reader", Code: "reader", Status: 1}
	roleDrop := authgorm.Role{Name: "Editor", Code: "editor", Status: 1}
	orgKeep := organization.Unit{Name: "Core", Code: "core", Status: 1}
	orgDrop := organization.Unit{Name: "Temp", Code: "temp", Status: 1}
	for _, item := range []any{&roleKeep, &roleDrop, &orgKeep, &orgDrop} {
		if err := env.db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []any{
		&authgorm.UserRole{UserID: env.target.ID, RoleID: roleKeep.ID},
		&authgorm.UserRole{UserID: env.target.ID, RoleID: roleDrop.ID},
		&organization.Membership{UserID: env.target.ID, OrganizationID: orgKeep.ID},
		&organization.Membership{UserID: env.target.ID, OrganizationID: orgDrop.ID},
	} {
		if err := env.db.Create(link).Error; err != nil {
			t.Fatal(err)
		}
	}

	pending := "pending-before-delete@example.test"
	if err := env.repo.Update(ctx, env.target.ID, application.UserChanges{PendingEmail: &pending}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	rawToken := bytes.Repeat([]byte{0x61}, 32)
	digest := sha256.Sum256(rawToken)
	if err := env.db.Create(&identitymodule.EmailVerificationCredential{
		UserID:    env.target.ID,
		Email:     pending,
		TokenHash: hex.EncodeToString(digest[:]),
		ExpiresAt: now.Add(15 * time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Simulate controlled cleanup of a role and an organization while user is soft-deleted.
	if err := env.db.Where("user_id = ? AND role_id = ?", env.target.ID, roleDrop.ID).Delete(&authgorm.UserRole{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := env.db.Where("user_id = ? AND organization_id = ?", env.target.ID, orgDrop.ID).Delete(&organization.Membership{}).Error; err != nil {
		t.Fatal(err)
	}

	restored, err := env.users.Restore(ctx, 1000, env.target.ID)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if restored.ID != env.target.ID || restored.Username != env.target.Username || restored.Email != env.target.Email || restored.Status != 1 {
		t.Fatalf("restored AdminUser = %#v", restored)
	}
	if restored.AccessVersion != 8 {
		t.Fatalf("restored AccessVersion = %d, want preserved post-delete version 8", restored.AccessVersion)
	}
	if len(restored.Roles) != 1 || restored.Roles[0].Code != "reader" {
		t.Fatalf("restored roles = %#v, want only retained reader role", restored.Roles)
	}
	if len(restored.Organizations) != 1 || restored.Organizations[0].Name != "Core" {
		t.Fatalf("restored orgs = %#v, want only retained Core org", restored.Organizations)
	}

	// Pre-delete email verification token remains invalid after Restore and pending_email stays empty.
	loaded, err := env.repo.FindByID(ctx, env.target.ID)
	if err != nil || loaded.PendingEmail != "" || loaded.EmailVerifiedAt == nil {
		t.Fatalf("loaded restored user = %#v, err = %v", loaded, err)
	}
	verification := application.NewEmailVerificationService(env.repo, platformdatabase.NewTransactionRunner(env.db), application.WithEmailVerificationClock(func() time.Time { return now }))
	if err := verification.Confirm(ctx, env.target.ID, base64.RawURLEncoding.EncodeToString(rawToken)); err == nil {
		t.Fatal("pre-delete email credential succeeded after Restore")
	} else if code, _ := application.CodeOf(err); code != application.CodeEmailVerificationInvalid {
		t.Fatalf("pre-delete email credential code = %q, want %q", code, application.CodeEmailVerificationInvalid)
	}

	// Pre-delete token version (7) stays rejected against the retained access version (8).
	authService := application.NewService(
		env.repo,
		&captchaFake{verified: true},
		passwordFake{},
		&loginAttemptFake{},
		newSessionFake(),
		snapshotAuthFake{snapshot: domain.AccessSnapshot{Version: restored.AccessVersion, Roles: []string{"reader"}}},
		&tokenFake{claims: application.TokenClaims{UserID: env.target.ID, TokenVersion: 7, Purpose: application.TokenPurposeAccess, SessionID: "s1", Generation: 1, RefreshID: "r1", ExpiresAt: time.Now().Add(time.Hour)}},
	)
	if _, err := authService.Authenticate(ctx, "stale-pre-delete-token"); err == nil {
		t.Fatal("pre-delete access token accepted after Restore")
	} else if code, _ := application.CodeOf(err); code != application.CodeTokenInvalid {
		t.Fatalf("stale token code = %q, want %q", code, application.CodeTokenInvalid)
	}
}

type snapshotAuthFake struct{ snapshot domain.AccessSnapshot }

func (fake snapshotAuthFake) EnsureVersion(context.Context, uint) (int, error) {
	return fake.snapshot.Version, nil
}

func (fake snapshotAuthFake) Snapshot(context.Context, uint) (domain.AccessSnapshot, error) {
	return fake.snapshot, nil
}

func TestRestoreRejectsInvalidStatesAndAuthorizationBoundaries(t *testing.T) {
	t.Run("missing self active and anonymized states", func(t *testing.T) {
		env := newRecoveryFixture(t)
		ctx := context.Background()

		if _, err := env.users.Restore(ctx, 1000, 99999); err == nil {
			t.Fatal("missing user restore succeeded")
		} else if code, _ := application.CodeOf(err); code != application.CodeUserNotFound {
			t.Fatalf("missing user code = %q, want %q", code, application.CodeUserNotFound)
		}
		if _, err := env.users.Restore(ctx, env.target.ID, env.target.ID); err == nil {
			t.Fatal("self restore succeeded")
		} else if code, _ := application.CodeOf(err); code != application.CodeValidationInvalid {
			t.Fatalf("self restore code = %q, want %q", code, application.CodeValidationInvalid)
		}
		if _, err := env.users.Restore(ctx, 1000, env.target.ID); err == nil {
			t.Fatal("active user restore succeeded")
		} else if code, _ := application.CodeOf(err); code != application.CodeConflict {
			t.Fatalf("active user restore code = %q, want %q", code, application.CodeConflict)
		}

		// Soft-delete then anonymize -> restore must return 409 conflict.
		if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
			t.Fatal(err)
		}
		if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err != nil {
			t.Fatal(err)
		}
		if _, err := env.users.Restore(ctx, 1000, env.target.ID); err == nil {
			t.Fatal("anonymized tombstone restore succeeded")
		} else if code, _ := application.CodeOf(err); code != application.CodeConflict {
			t.Fatalf("anonymized tombstone restore code = %q, want %q", code, application.CodeConflict)
		}
	})

	t.Run("non-all scope concurrent demotion and protected identity", func(t *testing.T) {
		env := newRecoveryFixture(t)
		ctx := context.Background()
		if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
			t.Fatal(err)
		}

		env.access.scope = domain.UserScope{All: false, UserIDs: []uint{env.target.ID}}
		if _, err := env.users.Restore(ctx, 1000, env.target.ID); err == nil {
			t.Fatal("non-all scope restore succeeded")
		} else if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
			t.Fatalf("non-all scope code = %q, want %q", code, application.CodePermissionDenied)
		}

		// Concurrent demotion between pre-tx check and in-tx locked check.
		env.access.scope = domain.UserScope{All: true}
		demoted := domain.UserScope{All: false, UserIDs: []uint{env.target.ID}}
		env.access.managementScope = &demoted
		if _, err := env.users.Restore(ctx, 1000, env.target.ID); err == nil {
			t.Fatal("concurrently demoted restore succeeded")
		} else if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
			t.Fatalf("concurrent demotion code = %q, want %q", code, application.CodePermissionDenied)
		}
		env.access.managementScope = nil

		// Protected admin identity (even disabled role) rejected at lock boundary.
		adminRole := authgorm.Role{Name: "Admin", Code: "admin", Status: 1}
		if err := env.db.Create(&adminRole).Error; err != nil {
			t.Fatal(err)
		}
		if err := env.db.Create(&authgorm.UserRole{UserID: env.target.ID, RoleID: adminRole.ID}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := env.users.Restore(ctx, 1000, env.target.ID); err == nil {
			t.Fatal("protected user restore succeeded")
		} else if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
			t.Fatalf("protected user restore code = %q, want %q", code, application.CodePermissionDenied)
		}
	})
}

func TestAnonymizeReleasesUniqueKeysClearsMembershipsAndDeletesTrustedAvatarPostCommit(t *testing.T) {
	env := newRecoveryFixture(t)
	ctx := context.Background()

	role := authgorm.Role{Name: "Member", Code: "member", Status: 1}
	org := organization.Unit{Name: "Engineering", Code: "eng", Status: 1}
	if err := env.db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := env.db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	if err := env.db.Create(&authgorm.UserRole{UserID: env.target.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := env.db.Create(&organization.Membership{UserID: env.target.ID, OrganizationID: org.ID}).Error; err != nil {
		t.Fatal(err)
	}

	trustedObject := fmt.Sprintf("avatars/%d/00000000-0000-4000-8000-000000000001.png", env.target.ID)
	validatedAt := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if err := env.db.Model(&identitygorm.UserModel{}).Where("id = ?", env.target.ID).Updates(map[string]any{
		"avatar":                   "legacy-ignored",
		"avatar_object_name":       trustedObject,
		"avatar_content_type":      "image/png",
		"avatar_content_sha256":    strings.Repeat("a", 64),
		"avatar_validation_status": domain.AvatarValidationStatusValidated,
		"avatar_validated_at":      &validatedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
		t.Fatal(err)
	}
	// Add a residual unused email credential to verify Anonymize invalidates any remaining unused rows.
	residualCred := identitymodule.EmailVerificationCredential{
		UserID:    env.target.ID,
		Email:     "residual@example.test",
		TokenHash: strings.Repeat("b", 64),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := env.db.Create(&residualCred).Error; err != nil {
		t.Fatal(err)
	}

	// Confirm unique keys are still occupied before anonymize.
	if conflict, err := env.repo.ExistsIdentityConflict(ctx, env.target.Username, "other@example.test"); err != nil || !conflict {
		t.Fatalf("pre-anonymize username conflict = %v, %v; want true", conflict, err)
	}
	if exists, err := env.repo.EmailExists(ctx, env.target.Email, 9999); err != nil || !exists {
		t.Fatalf("pre-anonymize email exists = %v, %v; want true", exists, err)
	}

	// Verify avatar deletion runs strictly AFTER the DB transaction has committed.
	var sawCommittedTombstone bool
	env.avatars.onDelete = func(bucket, object string) error {
		var check identitygorm.UserModel
		if err := env.db.Unscoped().First(&check, env.target.ID).Error; err != nil {
			return err
		}
		sawCommittedTombstone = check.AnonymizedAt != nil && check.AvatarObjectName == ""
		return nil
	}

	if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err != nil {
		t.Fatalf("Anonymize() error = %v", err)
	}
	if !sawCommittedTombstone {
		t.Fatal("avatar cleanup did not observe committed anonymized tombstone")
	}
	if len(env.avatars.calls) != 1 || env.avatars.calls[0].bucket != application.AvatarBucket || env.avatars.calls[0].object != trustedObject {
		t.Fatalf("avatar cleanup calls = %#v, want [%s/%s]", env.avatars.calls, application.AvatarBucket, trustedObject)
	}

	// Inspect tombstone row.
	var stored identitygorm.UserModel
	if err := env.db.Unscoped().First(&stored, env.target.ID).Error; err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("deleted_%d_", env.target.ID)
	if !stored.DeletedAt.Valid || stored.AnonymizedAt == nil ||
		!strings.HasPrefix(stored.Username, prefix) ||
		!strings.HasPrefix(stored.Email, prefix) || !strings.HasSuffix(stored.Email, "@deleted.invalid") ||
		stored.Password != "!" || stored.Nickname != "" || stored.Status != 0 ||
		stored.PendingEmail != "" || stored.EmailVerifiedAt != nil ||
		stored.Avatar != "" || stored.AvatarObjectName != "" || stored.AvatarContentType != "" ||
		stored.AvatarContentSHA256 != "" || stored.AvatarValidationStatus != "" || stored.AvatarValidatedAt != nil {
		t.Fatalf("tombstone fields invalid: %#v", stored)
	}

	var roleCount, orgCount int64
	_ = env.db.Model(&authgorm.UserRole{}).Where("user_id = ?", env.target.ID).Count(&roleCount).Error
	_ = env.db.Model(&organization.Membership{}).Where("user_id = ?", env.target.ID).Count(&orgCount).Error
	if roleCount != 0 || orgCount != 0 {
		t.Fatalf("memberships after anonymize = roles:%d orgs:%d, want 0/0", roleCount, orgCount)
	}
	var loadedCred identitymodule.EmailVerificationCredential
	if err := env.db.First(&loadedCred, residualCred.ID).Error; err != nil || loadedCred.UsedAt == nil {
		t.Fatalf("residual email credential after anonymize = %#v, err = %v", loadedCred, err)
	}
	version, err := authgorm.NewAccessVersions(env.db).Current(ctx, env.target.ID)
	if err != nil || version != 9 {
		t.Fatalf("retained access version = %d, %v; want 9", version, err)
	}

	// Management facts mark Deleted=true; public queries exclude tombstone.
	facts, err := env.repo.ManagementUsersByIDs(ctx, []uint{env.target.ID})
	if err != nil || len(facts) != 1 || !facts[0].Deleted {
		t.Fatalf("ManagementUsersByIDs(tombstone) = %#v, %v", facts, err)
	}
	if dir, err := env.repo.ListUsersByIDs(ctx, []uint{env.target.ID}); err != nil || len(dir) != 0 {
		t.Fatalf("ListUsersByIDs(tombstone) = %#v, %v", dir, err)
	}

	// Unique keys are immediately released for both new account creation and email rebind.
	if conflict, err := env.repo.ExistsIdentityConflict(ctx, env.target.Username, env.target.Email); err != nil || conflict {
		t.Fatalf("post-anonymize ExistsIdentityConflict = %v, %v; want false", conflict, err)
	}
	if exists, err := env.repo.EmailExists(ctx, env.target.Email, 9999); err != nil || exists {
		t.Fatalf("post-anonymize EmailExists = %v, %v; want false", exists, err)
	}
	// 1) Another active user can confirm email rebind to the released email.
	rebindUser := domain.User{Username: "rebinder", Email: "old-rebinder@example.test", PendingEmail: env.target.Email, Password: "hash", Role: "user", Status: 1}
	if err := env.repo.Create(ctx, &rebindUser); err != nil {
		t.Fatal(err)
	}
	rebindRaw := bytes.Repeat([]byte{0x77}, 32)
	rebindDigest := sha256.Sum256(rebindRaw)
	now := time.Now().UTC().Truncate(time.Second)
	if err := env.db.Create(&identitymodule.EmailVerificationCredential{
		UserID:    rebindUser.ID,
		Email:     env.target.Email,
		TokenHash: hex.EncodeToString(rebindDigest[:]),
		ExpiresAt: now.Add(15 * time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}
	verSvc := application.NewEmailVerificationService(env.repo, platformdatabase.NewTransactionRunner(env.db), application.WithEmailVerificationClock(func() time.Time { return now }))
	if err := verSvc.Confirm(ctx, rebindUser.ID, base64.RawURLEncoding.EncodeToString(rebindRaw)); err != nil {
		t.Fatalf("Confirm(released email) error = %v", err)
	}
	// 2) New user can be created with the released original username.
	reborn, err := env.users.CreateByAdmin(ctx, 1000, application.AdminCreateUserRequest{
		Username: env.target.Username,
		Password: "Password1!",
		Email:    "reborn-new-email@example.test",
		Nickname: "Reborn",
	})
	if err != nil || reborn.Username != env.target.Username {
		t.Fatalf("CreateByAdmin(released username) = %#v, %v", reborn, err)
	}
}

func TestAnonymizeRollsBackAtomicallyWithoutDeletingAvatarOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*recoveryFixtureEnv)
	}{
		{name: "version increment failure", setup: func(env *recoveryFixtureEnv) { env.access.versionErr = errors.New("version boom") }},
		{name: "organization cleanup failure", setup: func(env *recoveryFixtureEnv) { env.orgs.clearErr = errors.New("org boom") }},
		{name: "role cleanup failure", setup: func(env *recoveryFixtureEnv) { env.access.roleClearErr = errors.New("role boom") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newRecoveryFixture(t)
			ctx := context.Background()
			role := authgorm.Role{Name: "Member", Code: "member", Status: 1}
			org := organization.Unit{Name: "Engineering", Code: "eng", Status: 1}
			if err := env.db.Create(&role).Error; err != nil {
				t.Fatal(err)
			}
			if err := env.db.Create(&org).Error; err != nil {
				t.Fatal(err)
			}
			if err := env.db.Create(&authgorm.UserRole{UserID: env.target.ID, RoleID: role.ID}).Error; err != nil {
				t.Fatal(err)
			}
			if err := env.db.Create(&organization.Membership{UserID: env.target.ID, OrganizationID: org.ID}).Error; err != nil {
				t.Fatal(err)
			}
			trustedObject := fmt.Sprintf("avatars/%d/00000000-0000-4000-8000-000000000002.png", env.target.ID)
			if err := env.db.Model(&identitygorm.UserModel{}).Where("id = ?", env.target.ID).Updates(map[string]any{
				"avatar_object_name":       trustedObject,
				"avatar_content_type":      "image/png",
				"avatar_validation_status": domain.AvatarValidationStatusValidated,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
				t.Fatal(err)
			}
			cred := identitymodule.EmailVerificationCredential{
				UserID:    env.target.ID,
				Email:     "stay-unused@example.test",
				TokenHash: strings.Repeat("c", 64),
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			}
			if err := env.db.Create(&cred).Error; err != nil {
				t.Fatal(err)
			}

			tc.setup(env)
			if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err == nil {
				t.Fatal("Anonymize() succeeded despite injected tx failure")
			}
			if len(env.avatars.calls) != 0 {
				t.Fatalf("avatar cleanup ran on rolled-back tx: %#v", env.avatars.calls)
			}
			var stored identitygorm.UserModel
			if err := env.db.Unscoped().First(&stored, env.target.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.AnonymizedAt != nil || stored.Username != env.target.Username || stored.Email != env.target.Email || stored.AvatarObjectName != trustedObject {
				t.Fatalf("rolled-back user was mutated: %#v", stored)
			}
			var roleCount, orgCount int64
			_ = env.db.Model(&authgorm.UserRole{}).Where("user_id = ?", env.target.ID).Count(&roleCount).Error
			_ = env.db.Model(&organization.Membership{}).Where("user_id = ?", env.target.ID).Count(&orgCount).Error
			if roleCount != 1 || orgCount != 1 {
				t.Fatalf("rolled-back memberships = roles:%d orgs:%d, want 1/1", roleCount, orgCount)
			}
			var loadedCred identitymodule.EmailVerificationCredential
			if err := env.db.First(&loadedCred, cred.ID).Error; err != nil || loadedCred.UsedAt != nil {
				t.Fatalf("rolled-back credential mutated: %#v, err = %v", loadedCred, err)
			}
			if version, err := authgorm.NewAccessVersions(env.db).Current(ctx, env.target.ID); err != nil || version != 8 {
				t.Fatalf("rolled-back access version = %d, %v; want 8", version, err)
			}
		})
	}
}

func TestAnonymizeTrustedAvatarFailureDegradesAndLegacyAvatarNotDeleted(t *testing.T) {
	t.Run("trusted avatar delete failure keeps success and logs warning", func(t *testing.T) {
		env := newRecoveryFixture(t)
		ctx := context.Background()
		trustedObject := fmt.Sprintf("avatars/%d/00000000-0000-4000-8000-000000000003.png", env.target.ID)
		if err := env.db.Model(&identitygorm.UserModel{}).Where("id = ?", env.target.ID).Updates(map[string]any{
			"avatar_object_name":       trustedObject,
			"avatar_content_type":      "image/png",
			"avatar_validation_status": domain.AvatarValidationStatusValidated,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
			t.Fatal(err)
		}
		env.avatars.err = errors.New("minio unavailable for " + trustedObject)
		if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err != nil {
			t.Fatalf("Anonymize() failed on post-commit avatar delete error: %v", err)
		}
		if len(env.avatars.calls) != 1 {
			t.Fatalf("avatar delete calls = %d, want 1", len(env.avatars.calls))
		}
		if len(env.logger.failedUserIDs) != 1 || env.logger.failedUserIDs[0] != env.target.ID {
			t.Fatalf("logged failed user IDs = %v, want [%d]", env.logger.failedUserIDs, env.target.ID)
		}
		var stored identitygorm.UserModel
		if err := env.db.Unscoped().First(&stored, env.target.ID).Error; err != nil || stored.AnonymizedAt == nil {
			t.Fatalf("tombstone not persisted: %#v, err = %v", stored, err)
		}
	})

	t.Run("legacy unverified avatar URL is cleared in DB but never deleted from storage", func(t *testing.T) {
		env := newRecoveryFixture(t)
		ctx := context.Background()
		if err := env.db.Model(&identitygorm.UserModel{}).Where("id = ?", env.target.ID).Updates(map[string]any{
			"avatar":                   "https://legacy.cdn.example/u/old.jpg",
			"avatar_object_name":       "",
			"avatar_validation_status": "",
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
			t.Fatal(err)
		}
		if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err != nil {
			t.Fatalf("Anonymize() error = %v", err)
		}
		if len(env.avatars.calls) != 0 {
			t.Fatalf("legacy avatar triggered storage deletion: %#v", env.avatars.calls)
		}
		if len(env.logger.failedUserIDs) != 0 {
			t.Fatalf("unexpected cleanup warnings: %v", env.logger.failedUserIDs)
		}
		var stored identitygorm.UserModel
		if err := env.db.Unscoped().First(&stored, env.target.ID).Error; err != nil || stored.Avatar != "" {
			t.Fatalf("legacy avatar column not cleared: %#v, err = %v", stored, err)
		}
	})
}

func TestAnonymizeRejectsMismatchStatesAndAuthorizationBoundaries(t *testing.T) {
	env := newRecoveryFixture(t)
	ctx := context.Background()

	// Missing, self, active.
	if err := env.users.Anonymize(ctx, 1000, 99999, "anyone"); err == nil {
		t.Fatal("missing anonymize succeeded")
	} else if code, _ := application.CodeOf(err); code != application.CodeUserNotFound {
		t.Fatalf("missing code = %q, want %q", code, application.CodeUserNotFound)
	}
	if err := env.users.Anonymize(ctx, env.target.ID, env.target.ID, env.target.Username); err == nil {
		t.Fatal("self anonymize succeeded")
	} else if code, _ := application.CodeOf(err); code != application.CodeValidationInvalid {
		t.Fatalf("self code = %q, want %q", code, application.CodeValidationInvalid)
	}
	if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err == nil {
		t.Fatal("active anonymize succeeded")
	} else if code, _ := application.CodeOf(err); code != application.CodeConflict {
		t.Fatalf("active code = %q, want %q", code, application.CodeConflict)
	}

	// Soft-delete target.
	if err := env.users.Delete(ctx, 1000, env.target.ID); err != nil {
		t.Fatal(err)
	}

	// Mismatched confirm_username returns 422 with exact field error and does not mutate.
	err := env.users.Anonymize(ctx, 1000, env.target.ID, "wrong_username")
	if code, _ := application.CodeOf(err); code != application.CodeValidationInvalid {
		t.Fatalf("mismatch code = %q, want %q", code, application.CodeValidationInvalid)
	}
	details, ok := application.DetailsOf(err)
	if !ok || len(details.Fields) != 1 || details.Fields[0].Field != "confirm_username" || details.Fields[0].ErrorCode != "IDENTITY_CONFIRM_USERNAME_MISMATCH" {
		t.Fatalf("mismatch details = %#v", details)
	}

	// Non-all scope & concurrent demotion.
	env.access.scope = domain.UserScope{All: false, UserIDs: []uint{env.target.ID}}
	if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err == nil {
		t.Fatal("non-all anonymize succeeded")
	} else if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
		t.Fatalf("non-all code = %q, want %q", code, application.CodePermissionDenied)
	}
	env.access.scope = domain.UserScope{All: true}
	demoted := domain.UserScope{All: false}
	env.access.managementScope = &demoted
	if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err == nil {
		t.Fatal("concurrent demoted anonymize succeeded")
	} else if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
		t.Fatalf("concurrent demoted code = %q, want %q", code, application.CodePermissionDenied)
	}
	env.access.managementScope = nil

	// Protected admin identity rejected.
	adminRole := authgorm.Role{Name: "Admin", Code: "admin", Status: 1}
	if err := env.db.Create(&adminRole).Error; err != nil {
		t.Fatal(err)
	}
	link := authgorm.UserRole{UserID: env.target.ID, RoleID: adminRole.ID}
	if err := env.db.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err == nil {
		t.Fatal("protected anonymize succeeded")
	} else if code, _ := application.CodeOf(err); code != application.CodePermissionDenied {
		t.Fatalf("protected code = %q, want %q", code, application.CodePermissionDenied)
	}
	if err := env.db.Delete(&link).Error; err != nil {
		t.Fatal(err)
	}

	// Anonymize succeeds once, then repeat anonymize returns 409 conflict.
	if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err != nil {
		t.Fatalf("first anonymize error = %v", err)
	}
	if err := env.users.Anonymize(ctx, 1000, env.target.ID, env.target.Username); err == nil {
		t.Fatal("repeat anonymize succeeded")
	} else if code, _ := application.CodeOf(err); code != application.CodeConflict {
		t.Fatalf("repeat anonymize code = %q, want %q", code, application.CodeConflict)
	}
}
