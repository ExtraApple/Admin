package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	"admin/internal/identity"
	"admin/internal/navigation"
	"admin/internal/organization"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
)

// These tests use the production composition without App.New's external clients,
// seed data, global database, or HTTP authentication bypasses.
type atomicSyncFixture struct {
	db            *gorm.DB
	composition   navigationComposition
	authorization *authapplication.Service
}

func newAtomicSyncFixture(t *testing.T) atomicSyncFixture {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	models := append([]any{}, apigorm.Models()...)
	models = append(models, authgorm.Models()...)
	models = append(models, navigation.Models()...)
	models = append(models, identity.Models()...)
	models = append(models, organization.Models()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate atomic sync owners: %v", err)
	}
	resources := Resources{DB: db}
	users := newIdentityCore(resources)
	authorization := newAuthorizationService(resources, users.directory, users.repository)
	return atomicSyncFixture{
		db: db, authorization: authorization,
		composition: newNavigationComposition(resources, authorization, nil, users.directory),
	}
}

func atomicSyncCreate(t *testing.T, db *gorm.DB, records any) {
	t.Helper()
	if err := db.Create(records).Error; err != nil {
		t.Fatalf("create atomic sync fixture %T: %v", records, err)
	}
}

func TestAPIPermissionSyncCommitsAccurateMixedResults(t *testing.T) {
	fixture := newAtomicSyncFixture(t)
	permissions := []authgorm.Permission{
		{Model: gorm.Model{ID: 71}, Name: "Existing permission", Code: "atomic.existing", Group: "preserved", Sort: 9},
		{Model: gorm.Model{ID: 72}, Name: "Restorable permission", Code: "atomic.restored", Group: "preserved", Sort: 8},
	}
	atomicSyncCreate(t, fixture.db, &permissions)
	if err := fixture.db.Delete(&permissions[1]).Error; err != nil {
		t.Fatalf("soft-delete restorable permission: %v", err)
	}
	// Insert order differs from the business order (api_group, sort, id).
	apis := []apigorm.API{
		{Model: gorm.Model{ID: 15}, Name: "Shared explicit second", Method: "POST", Path: "/api/atomic/shared", Group: "b", Sort: 2, PermissionCode: "atomic.shared", NeedAuth: 1},
		{Model: gorm.Model{ID: 13}, Name: "Generated permission", Method: "GET", Path: "/api/atomic/generated", Group: "a", Sort: 2, NeedAuth: 1},
		{Model: gorm.Model{ID: 11}, Name: "Public API", Method: "GET", Path: "/api/atomic/public", Group: "a", Sort: 0, NeedAuth: 1},
		{Model: gorm.Model{ID: 16}, Name: "Restore existing permission", Method: "GET", Path: "/api/atomic/restored", Group: "b", Sort: 3, PermissionCode: "atomic.restored", NeedAuth: 1},
		{Model: gorm.Model{ID: 14}, Name: "Shared explicit first", Method: "GET", Path: "/api/atomic/shared", Group: "b", Sort: 1, PermissionCode: " atomic.shared ", NeedAuth: 1},
		{Model: gorm.Model{ID: 12}, Name: "Existing API", Method: "GET", Path: "/api/atomic/existing", Group: "a", Sort: 1, PermissionCode: "atomic.existing", NeedAuth: 1},
	}
	atomicSyncCreate(t, fixture.db, &apis)
	// GORM applies the model's default 1 to a zero-valued NeedAuth on insert.
	if err := fixture.db.Model(&apigorm.API{}).Where("id = ?", 11).Update("need_auth", 0).Error; err != nil {
		t.Fatalf("mark public API: %v", err)
	}

	created, updated, err := fixture.composition.APIService.SyncPermissions(context.Background())
	if err != nil {
		t.Fatalf("SyncPermissions: %v", err)
	}
	// The generated code is created inside ChangeAPIPermissionCode, not its
	// following EnsureAPIPermission. Shared explicit codes count only once;
	// restoring an existing soft-deleted row does not count as creation.
	if want := []string{"atomic.generated.get", "atomic.shared"}; !reflect.DeepEqual(created, want) || updated != 1 {
		t.Fatalf("sync results = (%v, %d), want (%v, 1)", created, updated, want)
	}
	facts := atomicSyncFacts(t, fixture.db)
	wantAPIs := []atomicSyncAPIFact{
		{ID: 11, NeedAuth: 0},
		{ID: 12, PermissionCode: "atomic.existing", NeedAuth: 1},
		{ID: 13, PermissionCode: "atomic.generated.get", NeedAuth: 1},
		{ID: 14, PermissionCode: " atomic.shared ", NeedAuth: 1},
		{ID: 15, PermissionCode: "atomic.shared", NeedAuth: 1},
		{ID: 16, PermissionCode: "atomic.restored", NeedAuth: 1},
	}
	if !reflect.DeepEqual(facts.APIs, wantAPIs) {
		t.Fatalf("committed APIs = %#v, want %#v", facts.APIs, wantAPIs)
	}
	wantPermissions := []atomicSyncPermissionFact{
		{ID: 71, Name: "Existing permission", Code: "atomic.existing", Group: "preserved", Sort: 9},
		{ID: 73, Name: "Generated permission", Code: "atomic.generated.get", Group: "a", Sort: 2},
		{ID: 72, Name: "Restorable permission", Code: "atomic.restored", Group: "preserved", Sort: 8},
		{ID: 74, Name: "Shared explicit first", Code: "atomic.shared", Group: "b", Sort: 1},
	}
	if !reflect.DeepEqual(facts.Permissions, wantPermissions) {
		t.Fatalf("committed permissions = %#v, want %#v", facts.Permissions, wantPermissions)
	}
	if len(facts.Versions) != 0 {
		t.Fatalf("permission-only sync invented affected users: %#v", facts.Versions)
	}

	created, updated, err = fixture.composition.APIService.SyncPermissions(context.Background())
	if err != nil || len(created) != 0 || updated != 0 {
		t.Fatalf("repeat sync = (%v, %d, %v), want no changes", created, updated, err)
	}
	if after := atomicSyncFacts(t, fixture.db); !reflect.DeepEqual(facts, after) {
		t.Fatalf("repeat sync changed committed facts: before=%#v after=%#v", facts, after)
	}
}

// Historical SQL bindings can contain two blank API codes on one direct menu.
// Sync preserves its original candidate order: A propagates to both APIs, then
// the second originally blank candidate changes the current A to B. This is not
// transitive propagation and must not be replaced with a different graph policy.
func newAtomicSyncGraphFixture(t *testing.T, sharedMenu, grantFirstCode bool) atomicSyncFixture {
	t.Helper()
	fixture := newAtomicSyncFixture(t)
	apis := []apigorm.API{
		{Model: gorm.Model{ID: 11}, Name: "First", Method: "GET", Path: "/api/atomic/first", Group: "a", Sort: 1, NeedAuth: 1},
		{Model: gorm.Model{ID: 12}, Name: "Second", Method: "GET", Path: "/api/atomic/second", Group: "a", Sort: 2, NeedAuth: 1},
		{Model: gorm.Model{ID: 13}, Name: "First linked", Method: "POST", Path: "/api/atomic/first-linked", Group: "z", PermissionCode: "atomic.linked.first", NeedAuth: 1},
		{Model: gorm.Model{ID: 14}, Name: "Second linked", Method: "POST", Path: "/api/atomic/second-linked", Group: "z", PermissionCode: "atomic.linked.second", NeedAuth: 1},
	}
	atomicSyncCreate(t, fixture.db, &apis)
	menus := []navigation.MenuModel{{Model: gorm.Model{ID: 31}, Name: "First action", PermissionCode: "atomic.menu.first", Type: 3}}
	if !sharedMenu {
		menus = append(menus, navigation.MenuModel{Model: gorm.Model{ID: 32}, Name: "Second action", PermissionCode: "atomic.menu.second", Type: 3})
	}
	atomicSyncCreate(t, fixture.db, &menus)
	secondMenu := uint(32)
	if sharedMenu {
		secondMenu = 31
	}
	atomicSyncCreate(t, fixture.db, &[]navigation.MenuAPIModel{
		{MenuID: 31, APIID: 11}, {MenuID: 31, APIID: 13},
		{MenuID: secondMenu, APIID: 12}, {MenuID: secondMenu, APIID: 14},
	})
	role := authgorm.Role{Model: gorm.Model{ID: 41}, Name: "Sync operator", Code: "sync-operator", Status: 1}
	atomicSyncCreate(t, fixture.db, &role)
	roleMenus := []navigation.RoleMenuModel{{RoleID: 41, MenuID: 31}}
	if !sharedMenu {
		roleMenus = append(roleMenus, navigation.RoleMenuModel{RoleID: 41, MenuID: 32})
	}
	atomicSyncCreate(t, fixture.db, &roleMenus)
	atomicSyncCreate(t, fixture.db, &[]identity.User{
		{Model: gorm.Model{ID: 51}, Username: "sync-existing", Password: "unused", Email: "sync-existing@test.local", Status: 1},
		{Model: gorm.Model{ID: 52}, Username: "sync-missing-version", Password: "unused", Email: "sync-missing-version@test.local", Status: 1},
		{Model: gorm.Model{ID: 53}, Username: "sync-unrelated", Password: "unused", Email: "sync-unrelated@test.local", Status: 1},
	})
	atomicSyncCreate(t, fixture.db, &[]authgorm.UserRole{{UserID: 51, RoleID: 41}, {UserID: 52, RoleID: 41}})
	atomicSyncCreate(t, fixture.db, &[]authgorm.UserAccessVersion{{UserID: 51, Version: 10}, {UserID: 53, Version: 20}})
	atomicSyncCreate(t, fixture.db, &[]authgorm.Permission{
		{Model: gorm.Model{ID: 71}, Name: "Keep grant", Code: "atomic.keep"},
		{Model: gorm.Model{ID: 72}, Name: "First linked", Code: "atomic.linked.first"},
		{Model: gorm.Model{ID: 73}, Name: "Second linked", Code: "atomic.linked.second"},
	})
	atomicSyncCreate(t, fixture.db, &authgorm.RolePermission{RoleID: 41, PermissionID: 71})
	if grantFirstCode {
		atomicSyncCreate(t, fixture.db, &authgorm.Permission{Model: gorm.Model{ID: 74}, Name: "First existing grant", Code: "atomic.first.get"})
		atomicSyncCreate(t, fixture.db, &authgorm.RolePermission{RoleID: 41, PermissionID: 74})
	}
	return fixture
}

func TestAPIPermissionSyncKeepsPerAPIInvalidationAndCountsFinalPermissions(t *testing.T) {
	for _, test := range []struct {
		name       string
		sharedMenu bool
		grantFirst bool
		created    []string
	}{
		{name: "disjoint changes invalidate the same user twice", created: []string{"atomic.first.get", "atomic.second.get"}},
		{name: "shared menu counts only surviving new permission", sharedMenu: true, created: []string{"atomic.second.get"}},
		{name: "shared menu migrates existing permission grants", sharedMenu: true, grantFirst: true, created: []string{"atomic.second.get"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAtomicSyncGraphFixture(t, test.sharedMenu, test.grantFirst)
			before := atomicSyncFacts(t, fixture.db)
			created, updated, err := fixture.composition.APIService.SyncPermissions(context.Background())
			if err != nil || !reflect.DeepEqual(created, test.created) || updated != 2 {
				t.Fatalf("sync = (%v, %d, %v), want (%v, 2, nil)", created, updated, err, test.created)
			}
			after := atomicSyncFacts(t, fixture.db)
			wantVersions := []atomicSyncVersionFact{{UserID: 51, Version: 12}, {UserID: 52, Version: 3}, {UserID: 53, Version: 20}}
			if !reflect.DeepEqual(after.Versions, wantVersions) {
				t.Fatalf("committed versions = %#v, want %#v", after.Versions, wantVersions)
			}
			firstCode := "atomic.first.get"
			if test.sharedMenu {
				firstCode = "atomic.second.get"
			}
			wantAPIs := []atomicSyncAPIFact{
				{ID: 11, PermissionCode: firstCode, NeedAuth: 1},
				{ID: 12, PermissionCode: "atomic.second.get", NeedAuth: 1},
				{ID: 13, PermissionCode: firstCode, NeedAuth: 1},
				{ID: 14, PermissionCode: "atomic.second.get", NeedAuth: 1},
			}
			if !reflect.DeepEqual(after.APIs, wantAPIs) {
				t.Fatalf("propagated APIs = %#v, want %#v", after.APIs, wantAPIs)
			}
			wantMenus := []atomicSyncMenuFact{{ID: 31, PermissionCode: firstCode}}
			if !test.sharedMenu {
				wantMenus = append(wantMenus, atomicSyncMenuFact{ID: 32, PermissionCode: "atomic.second.get"})
			}
			if !reflect.DeepEqual(after.Menus, wantMenus) {
				t.Fatalf("propagated menus = %#v, want %#v", after.Menus, wantMenus)
			}
			wantCodes := []string{"atomic.keep", "atomic.linked.first", "atomic.linked.second", "atomic.second.get"}
			if !test.sharedMenu {
				wantCodes = append([]string{"atomic.first.get"}, wantCodes...)
			}
			codes := make([]string, len(after.Permissions))
			for index, permission := range after.Permissions {
				codes[index] = permission.Code
				if permission.Deleted {
					t.Fatalf("sync left deleted permission: %#v", permission)
				}
			}
			if !reflect.DeepEqual(codes, wantCodes) {
				t.Fatalf("final permission codes = %v, want %v", codes, wantCodes)
			}
			grants, err := fixture.authorization.Permissions(context.Background(), 51)
			wantGrants := []string{"atomic.keep"}
			if test.grantFirst {
				wantGrants = append(wantGrants, "atomic.second.get")
			}
			if err != nil || !reflect.DeepEqual(grants, wantGrants) {
				t.Fatalf("user permission grants = (%v, %v), want %v", grants, err, wantGrants)
			}
			if !reflect.DeepEqual(before.MenuAPIs, after.MenuAPIs) || !reflect.DeepEqual(before.RoleMenus, after.RoleMenus) || !reflect.DeepEqual(before.UserRoles, after.UserRoles) {
				t.Fatalf("sync changed relation membership: before=%#v after=%#v", before, after)
			}
		})
	}
}

func TestAPIPermissionSyncRollsBackWholeRequestOnTerminalSecondChange(t *testing.T) {
	const injectedError = "atomic_sync_injected"
	for _, test := range []struct {
		name         string
		sharedGrants bool
		trigger      string
	}{
		{name: "second Core update", trigger: "BEFORE UPDATE OF permission_code ON apis WHEN NEW.id = 12 AND NEW.permission_code = 'atomic.second.get'"},
		{name: "second permission creation", trigger: "BEFORE INSERT ON permissions WHEN NEW.code = 'atomic.second.get'"},
		{name: "second menu code propagation", trigger: "BEFORE UPDATE OF permission_code ON menus WHEN NEW.id = 32 AND NEW.permission_code = 'atomic.second.get'"},
		{name: "second linked API propagation", trigger: "BEFORE UPDATE OF permission_code ON apis WHEN NEW.id = 14 AND NEW.permission_code = 'atomic.second.get'"},
		{name: "second grant insertion", sharedGrants: true, trigger: "BEFORE INSERT ON role_permissions WHEN NEW.role_id = 41 AND NEW.permission_id = (SELECT id FROM permissions WHERE code = 'atomic.second.get')"},
		{name: "second old grant removal", sharedGrants: true, trigger: "BEFORE DELETE ON role_permissions WHEN OLD.role_id = 41 AND OLD.permission_id = 74"},
		{name: "second late access version increment", trigger: "BEFORE UPDATE OF version ON user_access_versions WHEN NEW.user_id = 51 AND OLD.version = 11 AND NEW.version = 12"},
		{name: "second obsolete permission cleanup", sharedGrants: true, trigger: "BEFORE DELETE ON permissions WHEN OLD.code = 'atomic.first.get'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAtomicSyncGraphFixture(t, test.sharedGrants, test.sharedGrants)
			before := atomicSyncFacts(t, fixture.db)
			statement := fmt.Sprintf("CREATE TRIGGER atomic_sync_failure %s BEGIN SELECT RAISE(ABORT, '%s'); END", test.trigger, injectedError)
			if err := fixture.db.Exec(statement).Error; err != nil {
				t.Fatalf("install failure trigger: %v", err)
			}
			created, updated, err := fixture.composition.APIService.SyncPermissions(context.Background())
			if err == nil {
				t.Fatal("SyncPermissions succeeded despite terminal database failure")
			}
			foundInjectedError := false
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				if strings.Contains(cause.Error(), injectedError) {
					foundInjectedError = true
					break
				}
			}
			if !foundInjectedError {
				t.Fatalf("sync failed before reaching the intended database position: %v", err)
			}
			if created != nil || updated != 0 {
				t.Fatalf("failed request published partial statistics: created=%v updated=%d", created, updated)
			}
			if after := atomicSyncFacts(t, fixture.db); !reflect.DeepEqual(before, after) {
				t.Fatalf("failed request persisted partial facts:\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

type atomicSyncAPIFact struct {
	ID             uint
	PermissionCode string
	NeedAuth       int
	Deleted        bool
}

type atomicSyncMenuFact struct {
	ID             uint
	PermissionCode string
	Deleted        bool
}

type atomicSyncPermissionFact struct {
	ID      uint
	Name    string
	Code    string
	Group   string
	Sort    int
	Deleted bool
}

type atomicSyncVersionFact struct {
	UserID  uint
	Version int
}

type atomicSyncProjection struct {
	APIs            []atomicSyncAPIFact
	Menus           []atomicSyncMenuFact
	Permissions     []atomicSyncPermissionFact
	RolePermissions []authgorm.RolePermission
	MenuAPIs        []navigation.MenuAPIModel
	RoleMenus       []navigation.RoleMenuModel
	UserRoles       []authgorm.UserRole
	Versions        []atomicSyncVersionFact
}

// Compare authorization facts, not timestamps or unrelated GORM internals.
// Unscoped permissions distinguish rollback from an accidentally retained tombstone.
func atomicSyncFacts(t *testing.T, db *gorm.DB) atomicSyncProjection {
	t.Helper()
	var facts atomicSyncProjection
	queries := []struct {
		name string
		run  func() error
	}{
		{"APIs", func() error {
			return db.Unscoped().Model(&apigorm.API{}).Select("id, permission_code, need_auth, deleted_at IS NOT NULL AS deleted").Order("id").Scan(&facts.APIs).Error
		}},
		{"menus", func() error {
			return db.Unscoped().Model(&navigation.MenuModel{}).Select("id, permission_code, deleted_at IS NOT NULL AS deleted").Order("id").Scan(&facts.Menus).Error
		}},
		{"permissions", func() error {
			return db.Unscoped().Model(&authgorm.Permission{}).Select("id, name, code, `group`, sort, deleted_at IS NOT NULL AS deleted").Order("code").Scan(&facts.Permissions).Error
		}},
		{"role permissions", func() error {
			return db.Order("role_id, permission_id").Find(&facts.RolePermissions).Error
		}},
		{"menu APIs", func() error {
			return db.Order("menu_id, api_id").Find(&facts.MenuAPIs).Error
		}},
		{"role menus", func() error {
			return db.Order("role_id, menu_id").Find(&facts.RoleMenus).Error
		}},
		{"user roles", func() error {
			return db.Order("user_id, role_id").Find(&facts.UserRoles).Error
		}},
		{"access versions", func() error {
			return db.Model(&authgorm.UserAccessVersion{}).Select("user_id, version").Order("user_id").Scan(&facts.Versions).Error
		}},
	}
	for _, query := range queries {
		if err := query.run(); err != nil {
			t.Fatalf("read atomic sync %s: %v", query.name, err)
		}
	}
	return facts
}
