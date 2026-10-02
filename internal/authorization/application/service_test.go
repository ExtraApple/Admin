package application_test

import (
	"context"
	"testing"

	authgorm "admin/internal/authorization/adapters/gorm"
	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

type fixtureUserDirectory map[uint]identitydomain.DirectoryUser

func (directory fixtureUserDirectory) ListUsersByIDs(_ context.Context, userIDs []uint) ([]identitydomain.DirectoryUser, error) {
	users := make([]identitydomain.DirectoryUser, 0, len(userIDs))
	for _, userID := range userIDs {
		if user, ok := directory[userID]; ok {
			users = append(users, user)
		}
	}
	return users, nil
}

type authorizationFixture struct {
	service       *application.Service
	repository    *authgorm.Repository
	organizations organization.Repository
	versions      *authgorm.AccessVersions
	users         fixtureUserDirectory
	transactions  application.TransactionRunner
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
	users := make(fixtureUserDirectory)
	transactions := platformdatabase.NewTransactionRunner(db)
	service := application.NewService(
		repository,
		transactions,
		organization.NewScopeReader(organizationRepository, hierarchy),
		users,
		versions,
		nil,
	)
	return authorizationFixture{service: service, repository: repository, organizations: organizationRepository, versions: versions, users: users, transactions: transactions}
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
func TestGetRoleAndSetUserRolesChangeOnlyTargetAndAdvanceVersionOnce(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()

	admin := domain.Role{Name: "Admin", Code: "admin", Status: 1, DataScope: domain.DataScopeAll}
	reader := domain.Role{Name: "Reader", Code: "reader", Status: 1, DataScope: domain.DataScopeSelf}
	editor := domain.Role{Name: "Editor", Code: "editor", Status: 1, DataScope: domain.DataScopeSelf}
	for _, role := range []*domain.Role{&admin, &reader, &editor} {
		if err := fixture.repository.CreateRole(ctx, role); err != nil {
			t.Fatalf("create role %q: %v", role.Code, err)
		}
	}
	fixture.users[10] = identitydomain.DirectoryUser{ID: 10, Username: "operator"}
	fixture.users[20] = identitydomain.DirectoryUser{ID: 20, Username: "target"}
	fixture.users[30] = identitydomain.DirectoryUser{ID: 30, Username: "unrelated"}
	if err := fixture.repository.ReplaceRoleUsers(ctx, admin.ID, []uint{10}); err != nil {
		t.Fatalf("assign administrator role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, reader.ID, []uint{20, 30}); err != nil {
		t.Fatalf("assign reader role: %v", err)
	}
	if version, err := fixture.versions.EnsureAndIncrement(ctx, 20); err != nil || version != 2 {
		t.Fatalf("initialize target access version = %d, %v; want 2", version, err)
	}

	gotRole, err := fixture.service.GetRole(ctx, editor.ID)
	if err != nil || gotRole != editor {
		t.Fatalf("GetRole = %#v, %v; want %#v", gotRole, err, editor)
	}
	_, err = fixture.service.GetRole(ctx, 999)
	if code, ok := application.CodeOf(err); !ok || code != application.CodeNotFound {
		t.Fatalf("missing role error = %v; want AUTHZ_NOT_FOUND", err)
	}

	version, err := fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{
		RoleIDs: []uint{editor.ID}, ExpectedAccessVersion: 2,
	})
	if err != nil || version != 3 {
		t.Fatalf("SetUserRoles version = %d, %v; want 3", version, err)
	}
	targetRoleIDs, err := fixture.repository.RoleIDsByUser(ctx, 20)
	if err != nil || len(targetRoleIDs) != 1 || targetRoleIDs[0] != editor.ID {
		t.Fatalf("target role IDs = %#v, %v; want editor only", targetRoleIDs, err)
	}
	unrelatedRoleIDs, err := fixture.repository.RoleIDsByUser(ctx, 30)
	if err != nil || len(unrelatedRoleIDs) != 1 || unrelatedRoleIDs[0] != reader.ID {
		t.Fatalf("unrelated role IDs = %#v, %v; want reader unchanged", unrelatedRoleIDs, err)
	}
	if current, err := fixture.versions.Current(ctx, 20); err != nil || current != 3 {
		t.Fatalf("target access version = %d, %v; want one increment to 3", current, err)
	}

	_, err = fixture.service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{
		RoleIDs: []uint{reader.ID}, ExpectedAccessVersion: 2,
	})
	if code, ok := application.CodeOf(err); !ok || code != application.CodeConflict {
		t.Fatalf("stale write error = %v; want AUTHZ_CONFLICT", err)
	}
	targetRoleIDs, err = fixture.repository.RoleIDsByUser(ctx, 20)
	if err != nil || len(targetRoleIDs) != 1 || targetRoleIDs[0] != editor.ID {
		t.Fatalf("stale write changed target roles: %#v, %v", targetRoleIDs, err)
	}
	if current, err := fixture.versions.Current(ctx, 20); err != nil || current != 3 {
		t.Fatalf("version after stale write = %d, %v; want 3", current, err)
	}
}

func TestSetUserRolesRejectsProtectedTargetsAndInvalidAssignments(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	admin := domain.Role{Name: "Admin", Code: "admin", Status: 1, DataScope: domain.DataScopeAll}
	reader := domain.Role{Name: "Reader", Code: "reader", Status: 1, DataScope: domain.DataScopeSelf}
	for _, role := range []*domain.Role{&admin, &reader} {
		if err := fixture.repository.CreateRole(ctx, role); err != nil {
			t.Fatalf("create role %q: %v", role.Code, err)
		}
	}
	for _, userID := range []uint{10, 20, 30} {
		fixture.users[userID] = identitydomain.DirectoryUser{ID: userID}
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, admin.ID, []uint{10, 20}); err != nil {
		t.Fatalf("assign administrator role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, reader.ID, []uint{30}); err != nil {
		t.Fatalf("assign reader role: %v", err)
	}
	if version, err := fixture.versions.EnsureAndIncrement(ctx, 20); err != nil || version != 2 {
		t.Fatalf("initialize protected target version = %d, %v", version, err)
	}
	if version, err := fixture.versions.EnsureAndIncrement(ctx, 30); err != nil || version != 2 {
		t.Fatalf("initialize regular target version = %d, %v", version, err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, reader.ID, []uint{20, 30}); err != nil {
		t.Fatalf("assign reader role to protected target: %v", err)
	}

	for _, test := range []struct {
		name       string
		operatorID uint
		targetID   uint
		roleIDs    []uint
		wantCode   application.ErrorCode
	}{
		{name: "protected administrator target", operatorID: 10, targetID: 20, roleIDs: []uint{reader.ID}, wantCode: application.CodeConflict},
		{name: "self target", operatorID: 10, targetID: 10, roleIDs: []uint{reader.ID}, wantCode: application.CodeInvalidUser},
		{name: "assign administrator role", operatorID: 10, targetID: 30, roleIDs: []uint{admin.ID}, wantCode: application.CodeConflict},
		{name: "unknown role", operatorID: 10, targetID: 30, roleIDs: []uint{999}, wantCode: application.CodeValidationInvalid},
		{name: "zero role ID", operatorID: 10, targetID: 30, roleIDs: []uint{0}, wantCode: application.CodeValidationInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := fixture.repository.RoleIDsByUser(ctx, test.targetID)
			if err != nil {
				t.Fatalf("read target roles: %v", err)
			}
			_, err = fixture.service.SetUserRoles(ctx, test.operatorID, test.targetID, application.UpdateUserRolesRequest{
				RoleIDs: test.roleIDs, ExpectedAccessVersion: 2,
			})
			if got, ok := application.CodeOf(err); !ok || got != test.wantCode {
				t.Fatalf("SetUserRoles error code = %q, want %q (error %v)", got, test.wantCode, err)
			}
			after, err := fixture.repository.RoleIDsByUser(ctx, test.targetID)
			if err != nil {
				t.Fatalf("read target roles after rejection: %v", err)
			}
			if len(after) != len(before) {
				t.Fatalf("rejected write changed target roles: before=%v after=%v", before, after)
			}
			for index := range before {
				if after[index] != before[index] {
					t.Fatalf("rejected write changed target roles: before=%v after=%v", before, after)
				}
			}
		})
	}
}

type revokingScopeTransaction struct {
	base   application.TransactionRunner
	revoke func(context.Context) error
}

func (runner revokingScopeTransaction) Run(ctx context.Context, operation func(context.Context) error) error {
	if err := runner.revoke(ctx); err != nil {
		return err
	}
	return runner.base.Run(ctx, operation)
}
func TestSetUserRolesRevalidatesScopeAfterConcurrentOperatorRevocation(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	manager := domain.Role{Name: "Manager", Code: "manager", Status: 1, DataScope: domain.DataScopeAll}
	reader := domain.Role{Name: "Reader", Code: "reader", Status: 1, DataScope: domain.DataScopeSelf}
	for _, role := range []*domain.Role{&manager, &reader} {
		if err := fixture.repository.CreateRole(ctx, role); err != nil {
			t.Fatal(err)
		}
	}
	fixture.users[10] = identitydomain.DirectoryUser{ID: 10}
	fixture.users[20] = identitydomain.DirectoryUser{ID: 20}
	if err := fixture.repository.ReplaceUserRoles(ctx, 10, []uint{manager.ID}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.ReplaceUserRoles(ctx, 20, []uint{reader.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.versions.Ensure(ctx, 20); err != nil {
		t.Fatal(err)
	}
	transactions := revokingScopeTransaction{base: fixture.transactions, revoke: func(ctx context.Context) error { return fixture.repository.ReplaceUserRoles(ctx, 10, []uint{}) }}
	service := application.NewService(fixture.repository, transactions, organization.NewScopeReader(fixture.organizations, organization.NewHierarchy(fixture.organizations)), fixture.users, fixture.versions, nil)
	if _, err := service.SetUserRoles(ctx, 10, 20, application.UpdateUserRolesRequest{RoleIDs: []uint{}, ExpectedAccessVersion: 1}); err == nil {
		t.Fatal("revoked operator changed target roles")
	} else if code, _ := application.CodeOf(err); code != application.CodeInvalidUser {
		t.Fatalf("revoked scope code = %q", code)
	}
	roles, err := fixture.repository.RoleIDsByUser(ctx, 20)
	if err != nil || len(roles) != 1 || roles[0] != reader.ID {
		t.Fatalf("roles after revoked write = %v, %v", roles, err)
	}
	version, err := fixture.versions.Current(ctx, 20)
	if err != nil || version != 1 {
		t.Fatalf("version after revoked write = %d, %v", version, err)
	}
}
