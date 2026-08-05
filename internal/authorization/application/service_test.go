package application_test

import (
	"context"
	"testing"

	"admin/testsupport/testutil"
	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
)

type emptyUserDirectory struct{}

func (emptyUserDirectory) ListUsersByIDs(context.Context, []uint) ([]domain.UserSummary, error) {
	return []domain.UserSummary{}, nil
}

type authorizationFixture struct {
	service       *application.Service
	repository    *authgorm.Repository
	organizations organization.Repository
	versions      *authgorm.AccessVersions
}

func newAuthorizationFixture(t *testing.T) authorizationFixture {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	models := append(authgorm.Models(), organization.Models()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate authorization fixture: %v", err)
	}
	repository := authgorm.NewRepository(db)
	organizationRepository := organization.NewGORMRepository(db)
	hierarchy := organization.NewHierarchy(organizationRepository)
	versions := authgorm.NewAccessVersions(db)
	service := application.NewService(
		repository,
		platformdatabase.NewTransactionRunner(db),
		organization.NewScopeReader(organizationRepository, hierarchy),
		emptyUserDirectory{},
		versions,
	)
	return authorizationFixture{service: service, repository: repository, organizations: organizationRepository, versions: versions}
}

func TestResolveScopesPreservesSelfEmptyAndAllSemantics(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	self := domain.Role{Name: "Self", Code: "self", Status: 1, DataScope: domain.DataScopeSelf}
	if err := fixture.repository.CreateRole(ctx, &self); err != nil {
		t.Fatalf("create self role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, self.ID, []uint{7}); err != nil {
		t.Fatalf("assign self role: %v", err)
	}

	userScope, err := fixture.service.ResolveUserScope(ctx, domain.Principal{UserID: 7})
	if err != nil || userScope.All || len(userScope.UserIDs) != 1 || userScope.UserIDs[0] != 7 {
		t.Fatalf("self user scope = %#v, %v", userScope, err)
	}
	organizationScope, err := fixture.service.ResolveOrganizationScope(ctx, domain.Principal{UserID: 7})
	if err != nil || organizationScope.All || len(organizationScope.OrganizationIDs) != 0 {
		t.Fatalf("self organization scope = %#v, %v", organizationScope, err)
	}

	admin := domain.Role{Name: "Admin", Code: "admin", Status: 1, DataScope: domain.DataScopeSelf}
	if err := fixture.repository.CreateRole(ctx, &admin); err != nil {
		t.Fatalf("create admin role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, admin.ID, []uint{8}); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	userScope, err = fixture.service.ResolveUserScope(ctx, domain.Principal{UserID: 8})
	if err != nil || !userScope.All || len(userScope.UserIDs) != 0 {
		t.Fatalf("admin user scope = %#v, %v", userScope, err)
	}
	organizationScope, err = fixture.service.ResolveOrganizationScope(ctx, domain.Principal{UserID: 8})
	if err != nil || !organizationScope.All || len(organizationScope.OrganizationIDs) != 0 {
		t.Fatalf("admin organization scope = %#v, %v", organizationScope, err)
	}
}

func TestResolveScopesUsesOrganizationHierarchyContract(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	parent := organization.Unit{Name: "Parent", Code: "parent", Status: 1}
	if err := fixture.organizations.CreateUnit(ctx, &parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child := organization.Unit{Name: "Child", Code: "child", ParentID: parent.ID, Status: 1}
	if err := fixture.organizations.CreateUnit(ctx, &child); err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := fixture.organizations.CreateMemberships(ctx, []organization.Membership{{UserID: 10, OrganizationID: parent.ID}, {UserID: 11, OrganizationID: child.ID}}); err != nil {
		t.Fatalf("create memberships: %v", err)
	}
	role := domain.Role{Name: "Org Children", Code: "org-children", Status: 1, DataScope: domain.DataScopeOrgAndChildren}
	if err := fixture.repository.CreateRole(ctx, &role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, role.ID, []uint{10}); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	organizationScope, err := fixture.service.ResolveOrganizationScope(ctx, domain.Principal{UserID: 10})
	if err != nil || organizationScope.All || len(organizationScope.OrganizationIDs) != 2 {
		t.Fatalf("organization hierarchy scope = %#v, %v", organizationScope, err)
	}
	userScope, err := fixture.service.ResolveUserScope(ctx, domain.Principal{UserID: 10})
	if err != nil || userScope.All || len(userScope.UserIDs) != 2 || userScope.UserIDs[0] != 10 || userScope.UserIDs[1] != 11 {
		t.Fatalf("organization hierarchy user scope = %#v, %v", userScope, err)
	}
}

func TestSnapshotReturnsRuntimeRolesPermissionsAndAccessVersion(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	role := domain.Role{Name: "Reader", Code: "reader", Status: 1, DataScope: domain.DataScopeAll}
	if err := fixture.repository.CreateRole(ctx, &role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	code, _ := domain.NewPermissionCode("document.read")
	permission := domain.Permission{Name: "Read", Code: code}
	if err := fixture.repository.CreatePermission(ctx, &permission); err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, role.ID, []uint{21}); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	if err := fixture.repository.ReplaceRolePermissions(ctx, role.ID, []uint{permission.ID}); err != nil {
		t.Fatalf("assign permission: %v", err)
	}
	if _, err := fixture.versions.EnsureAndIncrement(ctx, 21); err != nil {
		t.Fatalf("initialize access version: %v", err)
	}

	snapshot, err := fixture.service.Snapshot(ctx, domain.Principal{UserID: 21})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Principal.UserID != 21 || len(snapshot.Roles) != 1 || snapshot.Roles[0] != "reader" || len(snapshot.Permissions) != 1 || snapshot.Permissions[0] != "document.read" || snapshot.Version != 2 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
func TestSyncPermissionsUsesRouteFactsAndSkipsPublicRoutes(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	result, err := fixture.service.SyncPermissions(context.Background(), []domain.RouteFact{
		{Method: "GET", Path: "/api/login", Public: true},
		{Method: "GET", Path: "/api/admin/reports/:id", PermissionCode: "admin.reports.id.get"},
		{Method: "POST", Path: "/api/admin/generated"},
	})
	if err != nil {
		t.Fatalf("sync permissions: %v", err)
	}
	if len(result.Created) != 2 || result.Created[0] != "admin.reports.id.get" || result.Created[1] != "admin.generated.post" {
		t.Fatalf("created permissions = %#v", result.Created)
	}
	result, err = fixture.service.SyncPermissions(context.Background(), []domain.RouteFact{{Method: "POST", Path: "/api/admin/generated"}})
	if err != nil {
		t.Fatalf("repeat sync permissions: %v", err)
	}
	if len(result.Created) != 0 {
		t.Fatalf("repeat sync created = %#v, want empty", result.Created)
	}
}
func TestRolePermissionAndDataScopeChangesInvalidateAccessVersion(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	unit := organization.Unit{Name: "Scoped", Code: "scoped", Status: 1}
	if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
		t.Fatalf("create scoped organization: %v", err)
	}
	role := domain.Role{Name: "Managed", Code: "managed", Status: 1, DataScope: domain.DataScopeSelf}
	if err := fixture.repository.CreateRole(ctx, &role); err != nil {
		t.Fatalf("create managed role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, role.ID, []uint{30}); err != nil {
		t.Fatalf("assign managed role: %v", err)
	}
	if version, err := fixture.versions.EnsureAndIncrement(ctx, 30); err != nil || version != 2 {
		t.Fatalf("initialize version = %d, %v", version, err)
	}
	sortOrder := 9
	if _, err := fixture.service.UpdateRole(ctx, role.ID, application.UpdateRoleRequest{Sort: &sortOrder}); err != nil {
		t.Fatalf("update role: %v", err)
	}
	if version, _ := fixture.versions.Current(ctx, 30); version != 3 {
		t.Fatalf("version after role update = %d, want 3", version)
	}
	if err := fixture.service.AssignRoleDataScope(ctx, role.ID, string(domain.DataScopeCustom), []uint{unit.ID}); err != nil {
		t.Fatalf("assign role data scope: %v", err)
	}
	if version, _ := fixture.versions.Current(ctx, 30); version != 4 {
		t.Fatalf("version after data scope update = %d, want 4", version)
	}
	code, _ := domain.NewPermissionCode("managed.read")
	permission := domain.Permission{Name: "Managed Read", Code: code}
	if err := fixture.repository.CreatePermission(ctx, &permission); err != nil {
		t.Fatalf("create managed permission: %v", err)
	}
	if err := fixture.service.AssignPermissionsToRole(ctx, role.ID, []uint{permission.ID}); err != nil {
		t.Fatalf("assign role permissions: %v", err)
	}
	if version, _ := fixture.versions.Current(ctx, 30); version != 5 {
		t.Fatalf("version after permission assignment = %d, want 5", version)
	}
	if err := fixture.service.DeletePermission(ctx, permission.ID); err != nil {
		t.Fatalf("delete permission: %v", err)
	}
	if version, _ := fixture.versions.Current(ctx, 30); version != 6 {
		t.Fatalf("version after permission deletion = %d, want 6", version)
	}
}

func TestSuperAdministratorRoleIsProtectedByApplicationUseCases(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	admin := domain.Role{Name: "Admin", Code: "admin", Status: 1, DataScope: domain.DataScopeAll}
	if err := fixture.repository.CreateRole(ctx, &admin); err != nil {
		t.Fatalf("create admin role: %v", err)
	}
	name := "Changed"
	if _, err := fixture.service.UpdateRole(ctx, admin.ID, application.UpdateRoleRequest{Name: name}); err == nil {
		t.Fatal("admin role update was allowed")
	}
	if err := fixture.service.DeleteRole(ctx, admin.ID); err == nil {
		t.Fatal("admin role deletion was allowed")
	}
	if err := fixture.service.AssignRoleDataScope(ctx, admin.ID, string(domain.DataScopeSelf), nil); err == nil {
		t.Fatal("admin data scope update was allowed")
	}
}
