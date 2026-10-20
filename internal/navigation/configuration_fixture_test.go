package navigation_test

import (
	"context"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/navigation"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
)

// Role/API facts stay local projections; invalidation uses its real transactional
// owner so assertions observe persisted versions, not captured method calls.
type configurationAuthorization struct {
	navigation.Authorization
	versions *authgorm.AccessVersions
}

func (authorization configurationAuthorization) IncrementAccessVersions(ctx context.Context, userIDs []uint) error {
	for _, userID := range userIDs {
		if _, err := authorization.versions.EnsureAndIncrement(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

type configurationFixture struct {
	service       *navigation.Service
	authorization *navigationAuthorizationFake
	apis          *navigationAPIFake
	versions      *authgorm.AccessVersions
	db            *gorm.DB
}

func newConfigurationNavigationFixture(t *testing.T) configurationFixture {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(append(navigation.Models(), &authgorm.UserAccessVersion{})...); err != nil {
		t.Fatal(err)
	}
	authorization := &navigationAuthorizationFake{
		allUsers: []uint{10, 20}, access: make(map[uint]navigation.UserAccess),
		roleUsers: make(map[uint][]uint), roles: make(map[uint]bool),
		permissions: make(map[string]navigation.PermissionRef), permissionRoles: make(map[uint][]uint),
	}
	apis := &navigationAPIFake{records: make(map[uint]navigation.APIRecord)}
	versions := authgorm.NewAccessVersions(db)
	capability := configurationAuthorization{Authorization: authorization, versions: versions}
	service := navigation.NewService(navigation.NewGORMRepository(db), platformdatabase.NewTransactionRunner(db), capability, apis)
	return configurationFixture{service: service, authorization: authorization, apis: apis, versions: versions, db: db}
}

func configurationVersions(t *testing.T, db *gorm.DB) map[uint]int {
	t.Helper()
	var rows []authgorm.UserAccessVersion
	if err := db.Order("user_id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	versions := make(map[uint]int, len(rows))
	for _, row := range rows {
		versions[row.UserID] = row.Version
	}
	return versions
}
