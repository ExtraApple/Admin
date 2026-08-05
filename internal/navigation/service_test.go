package navigation_test

import (
	"context"
	"errors"
	"testing"

	"admin/testsupport/testutil"
	"admin/internal/navigation"
	platformdatabase "admin/internal/platform/database"
)

func TestServiceCreatesAndReturnsOrderedMenuTree(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()

	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Settings", Path: "/settings", Sort: 2})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{ParentID: parent.ID, Name: "Users", Path: "/settings/users", Sort: 1, Type: 2})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Duplicate", Path: "/settings/users"}); err == nil {
		t.Fatal("duplicate menu path was accepted")
	}

	tree, err := service.MenuTree(ctx)
	if err != nil {
		t.Fatalf("menu tree: %v", err)
	}
	if len(tree) != 1 || tree[0].ID != parent.ID || tree[0].Status != 1 || tree[0].Type != 1 {
		t.Fatalf("root tree = %#v", tree)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].ID != child.ID {
		t.Fatalf("child tree = %#v", tree[0].Children)
	}
	if len(authorization.incremented) != 2 || len(authorization.incremented[0]) != 2 {
		t.Fatalf("access-version invalidations = %#v", authorization.incremented)
	}
}
func TestServiceUpdatesAndDeletesOnlyLeafMenus(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Parent", Path: "/parent"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{ParentID: parent.ID, Name: "Child", Path: "/child"})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := service.UpdateMenu(ctx, child.ID, navigation.UpdateInput{ParentID: &child.ID}); err == nil {
		t.Fatal("self parent was accepted")
	}
	if _, err := service.UpdateMenu(ctx, child.ID, navigation.UpdateInput{Path: "/parent"}); err == nil {
		t.Fatal("duplicate update path was accepted")
	}
	sort := 7
	updated, err := service.UpdateMenu(ctx, child.ID, navigation.UpdateInput{Name: "Renamed", Sort: &sort})
	if err != nil || updated.Name != "Renamed" || updated.Sort != sort {
		t.Fatalf("updated menu = %#v, %v", updated, err)
	}
	if err := service.DeleteMenu(ctx, parent.ID); err == nil {
		t.Fatal("parent with child was deleted")
	}
	if err := service.DeleteMenu(ctx, child.ID); err != nil {
		t.Fatalf("delete child: %v", err)
	}
	if err := service.DeleteMenu(ctx, parent.ID); err != nil {
		t.Fatalf("delete parent: %v", err)
	}
	if tree, err := service.MenuTree(ctx); err != nil || len(tree) != 0 {
		t.Fatalf("tree after delete = %#v, %v", tree, err)
	}
	if len(authorization.incremented) != 5 {
		t.Fatalf("access-version invalidation count = %d, want 5", len(authorization.incremented))
	}
}
func TestServiceAssignsRoleMenusAndPreservesVisibleParentChain(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()
	root, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Root", Path: "/root", PermissionCode: "root.hidden"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{ParentID: root.ID, Name: "Child", Path: "/root/child", PermissionCode: "child.read", Type: 2})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	authorization.roles[7] = true
	authorization.roleUsers[7] = []uint{42}
	authorization.access[42] = navigation.UserAccess{RoleIDs: []uint{7}, Permissions: []string{"child.read"}}
	if err := service.AssignRoleMenus(ctx, 7, []uint{child.ID, child.ID}); err != nil {
		t.Fatalf("assign role menus: %v", err)
	}
	if err := service.AssignRoleMenus(ctx, 99, []uint{child.ID}); err == nil {
		t.Fatal("missing role accepted menu assignment")
	}
	roleTree, err := service.RoleMenus(ctx, 7)
	if err != nil || len(roleTree) != 0 {
		t.Fatalf("role tree with unassigned parent = %#v, %v; want empty rooted tree", roleTree, err)
	}
	userTree, err := service.UserMenus(ctx, 42)
	if err != nil {
		t.Fatalf("user menus: %v", err)
	}
	if len(userTree) != 1 || userTree[0].ID != root.ID || len(userTree[0].Children) != 1 || userTree[0].Children[0].ID != child.ID {
		t.Fatalf("visible user tree = %#v", userTree)
	}
	lastInvalidation := authorization.incremented[len(authorization.incremented)-1]
	if len(lastInvalidation) != 1 || lastInvalidation[0] != 42 {
		t.Fatalf("role assignment invalidated users = %#v, want [42]", lastInvalidation)
	}
}
func TestServiceSyncsFrontendRoutesAndListsBoundAPIs(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(navigation.Models()...); err != nil {
		t.Fatalf("migrate navigation: %v", err)
	}
	repository := navigation.NewGORMRepository(db)
	authorization := &navigationAuthorizationFake{allUsers: []uint{10}, access: map[uint]navigation.UserAccess{}, roleUsers: map[uint][]uint{}, roles: map[uint]bool{}}
	apis := navigationAPIFake{records: map[uint]navigation.APIRecord{9: {ID: 9, Name: "Read Users", Method: "GET", Path: "/api/admin/users", Status: 1, NeedAuth: 1}}}
	service := navigation.NewService(repository, platformdatabase.NewTransactionRunner(db), authorization, &apis)
	ctx := context.Background()

	created, err := service.SyncMenus(ctx, []navigation.SyncItem{
		{Name: "Admin", Path: "/admin", Sort: 1},
		{Name: "Users", Path: "/admin/users", ParentPath: "/admin", Component: "views/users", Sort: 2, Type: 2},
		{Name: "Duplicate", Path: "/admin/users"},
	})
	if err != nil || created != 2 {
		t.Fatalf("SyncMenus created = %d, %v; want 2", created, err)
	}
	tree, err := service.MenuTree(ctx)
	if err != nil || len(tree) != 1 || len(tree[0].Children) != 1 {
		t.Fatalf("synced tree = %#v, %v", tree, err)
	}
	childID := tree[0].Children[0].ID
	if err := repository.ReplaceMenuAPIs(ctx, childID, []uint{9}); err != nil {
		t.Fatalf("prepare menu API relation: %v", err)
	}
	bound, err := service.MenuAPIs(ctx, childID)
	if err != nil || len(bound) != 1 || bound[0].ID != 9 {
		t.Fatalf("bound APIs = %#v, %v", bound, err)
	}
	empty, err := service.MenuAPIs(ctx, tree[0].ID)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("unbound APIs = %#v, %v; want non-nil empty", empty, err)
	}
}
func TestServiceAssignsAPIsWithOnePermissionAndInvalidatesAffectedUsers(t *testing.T) {
	service, authorization, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "User Actions", Type: 3})
	if err != nil {
		t.Fatalf("create menu: %v", err)
	}
	authorization.roles[7] = true
	authorization.roleUsers[7] = []uint{42}
	if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID}); err != nil {
		t.Fatalf("assign role menu: %v", err)
	}
	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Read Users", Method: "GET", Path: "/api/admin/users", Group: "user", Status: 1, NeedAuth: 1}
	authorization.incremented = nil
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9, 9}, "users.read"); err != nil {
		t.Fatalf("assign APIs: %v", err)
	}
	tree, err := service.MenuTree(ctx)
	if err != nil || len(tree) != 1 || tree[0].PermissionCode != "users.read" {
		t.Fatalf("menu after binding = %#v, %v", tree, err)
	}
	bound, err := service.MenuAPIs(ctx, menu.ID)
	if err != nil || len(bound) != 1 || bound[0].PermissionCode != "users.read" {
		t.Fatalf("bound APIs = %#v, %v", bound, err)
	}
	if _, exists := authorization.permissions["users.read"]; !exists {
		t.Fatal("permission was not ensured")
	}
	if len(authorization.incremented) != 1 || len(authorization.incremented[0]) != 1 || authorization.incremented[0][0] != 42 {
		t.Fatalf("affected user invalidations = %#v", authorization.incremented)
	}
}

func TestServiceRollsBackLinkedWritesWhenAuthorizationVersionFails(t *testing.T) {
	service, authorization, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Actions", Type: 3})
	if err != nil {
		t.Fatalf("create menu: %v", err)
	}
	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Update User", Method: "PUT", Path: "/api/admin/users/:id", Status: 1, NeedAuth: 1}
	authorization.incrementErr = errors.New("version failure")
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "users.write"); err == nil {
		t.Fatal("AssignAPIs succeeded despite version failure")
	}
	bound, err := service.MenuAPIs(ctx, menu.ID)
	if err != nil {
		t.Fatalf("read rolled back bindings: %v", err)
	}
	if len(bound) != 0 {
		t.Fatalf("menu API bindings after rollback = %#v", bound)
	}

	authorization.incrementErr = nil
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "users.old"); err != nil {
		t.Fatalf("prepare API binding: %v", err)
	}
	authorization.incrementErr = errors.New("version failure")
	if err := service.ChangeAPIPermissionCode(ctx, 9, "users.new", func(context.Context) error { return nil }); err == nil {
		t.Fatal("ChangeAPIPermissionCode succeeded despite version failure")
	}
	menuTree, err := service.MenuTree(ctx)
	if err != nil || len(menuTree) == 0 || menuTree[0].PermissionCode != "users.old" {
		t.Fatalf("menu permission after rollback = %#v, %v", menuTree, err)
	}

	authorization.incrementErr = nil
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Parent"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	authorization.incrementErr = errors.New("version failure")
	if _, err := service.GenerateMenuButton(ctx, 9, parent.ID, "Edit", 1); err == nil {
		t.Fatal("GenerateMenuButton succeeded despite version failure")
	}
	tree, err := service.MenuTree(ctx)
	if err != nil {
		t.Fatalf("read rolled back menus: %v", err)
	}
	for _, item := range tree {
		if item.Name == "Edit" {
			t.Fatal("button menu remained after rollback")
		}
	}
}
func TestServiceChangesLinkedAPIPermissionAndMergesRoleGrants(t *testing.T) {
	service, authorization, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Actions", PermissionCode: "users.old", Type: 3})
	if err != nil {
		t.Fatalf("create menu: %v", err)
	}
	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Update User", Method: "PUT", Path: "/api/admin/users/:id", Group: "user", PermissionCode: "users.old", Status: 1, NeedAuth: 1}
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "users.old"); err != nil {
		t.Fatalf("prepare API binding: %v", err)
	}
	oldPermission := authorization.permissions["users.old"]
	newPermission, _, err := authorization.EnsurePermission(ctx, navigation.PermissionSeed{Code: "users.write", Name: "Write", Group: "user"})
	if err != nil {
		t.Fatalf("prepare new permission: %v", err)
	}
	authorization.permissionRoles[oldPermission.ID] = []uint{7}
	authorization.permissionRoles[newPermission.ID] = []uint{7, 8}
	authorization.roleUsers[7] = []uint{42}
	authorization.roleUsers[8] = []uint{84}
	authorization.incremented = nil

	err = service.ChangeAPIPermissionCode(ctx, 9, "users.write", func(transactionContext context.Context) error {
		return apis.SetPermissionCode(transactionContext, []uint{9}, "users.write")
	})
	if err != nil {
		t.Fatalf("change API permission: %v", err)
	}
	tree, err := service.MenuTree(ctx)
	if err != nil || len(tree) != 1 || tree[0].PermissionCode != "users.write" || apis.records[9].PermissionCode != "users.write" {
		t.Fatalf("linked permission codes tree=%#v api=%#v err=%v", tree, apis.records[9], err)
	}
	if roles := authorization.permissionRoles[newPermission.ID]; len(roles) != 2 || roles[0] != 7 || roles[1] != 8 {
		t.Fatalf("merged role grants = %#v", roles)
	}
	if _, exists := authorization.permissions["users.old"]; exists {
		t.Fatal("unreferenced old permission was retained")
	}
	invalidated := authorization.incremented[0]
	if len(invalidated) != 2 || invalidated[0] != 42 || invalidated[1] != 84 {
		t.Fatalf("invalidated users = %#v", invalidated)
	}
}
func TestServiceGeneratesButtonMenuFromAuthenticatedAPI(t *testing.T) {
	service, authorization, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Users", Path: "/users"})
	if err != nil {
		t.Fatalf("create parent menu: %v", err)
	}
	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Update User", Method: "PUT", Path: "/api/admin/users/:id", Group: "user", Status: 1, NeedAuth: 1}
	button, err := service.GenerateMenuButton(ctx, 9, parent.ID, "Edit", 3)
	if err != nil {
		t.Fatalf("generate button: %v", err)
	}
	if button.ParentID != parent.ID || button.Type != 3 || button.PermissionCode != "admin.users.id.put" || button.Status != 1 {
		t.Fatalf("button = %#v", button)
	}
	if apis.records[9].PermissionCode != button.PermissionCode {
		t.Fatalf("API permission code = %q, want %q", apis.records[9].PermissionCode, button.PermissionCode)
	}
	if _, exists := authorization.permissions[button.PermissionCode]; !exists {
		t.Fatal("button permission was not ensured")
	}
	bound, err := service.MenuAPIs(ctx, button.ID)
	if err != nil || len(bound) != 1 || bound[0].ID != 9 {
		t.Fatalf("button API bindings = %#v, %v", bound, err)
	}
	apis.records[10] = navigation.APIRecord{ID: 10, Name: "Public", Method: "GET", Path: "/api/public", Status: 1, NeedAuth: 0}
	if _, err := service.GenerateMenuButton(ctx, 10, parent.ID, "Public", 4); err == nil {
		t.Fatal("public API generated a permission button")
	}
}
func TestServiceInvalidatesCacheAfterCommitAndExposesRetry(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(navigation.Models()...); err != nil {
		t.Fatalf("migrate navigation: %v", err)
	}
	runner := &orderingTransactionRunner{}
	authorization := &navigationAuthorizationFake{
		allUsers: []uint{42}, access: map[uint]navigation.UserAccess{},
		roleUsers: map[uint][]uint{7: {42}}, roles: map[uint]bool{7: true},
		permissions: map[string]navigation.PermissionRef{}, permissionRoles: map[uint][]uint{},
	}
	apis := &navigationAPIFake{records: map[uint]navigation.APIRecord{9: {ID: 9, Name: "Read", Method: "GET", Path: "/api/admin/users", Status: 1, NeedAuth: 1}}}
	cache := &cacheInvalidatorFake{runner: runner, err: errors.New("cache unavailable")}
	metrics := &cacheMetricsFake{}
	service := navigation.NewService(navigation.NewGORMRepository(db), runner, authorization, apis, navigation.WithCacheInvalidation(cache, metrics))
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Actions", Type: 3})
	if err != nil {
		t.Fatalf("create menu: %v", err)
	}
	if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID}); err != nil {
		t.Fatalf("assign role menu: %v", err)
	}
	cache.calls, metrics.failures = 0, 0
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "users.read"); err != nil {
		t.Fatalf("assign APIs despite cache failure: %v", err)
	}
	if cache.calls != 1 || cache.calledInsideTransaction || metrics.failures != 1 {
		t.Fatalf("post-commit cache calls=%d inside=%v failures=%d", cache.calls, cache.calledInsideTransaction, metrics.failures)
	}
	cache.err = nil
	if err := service.RetryCacheInvalidation(ctx, []uint{42}); err != nil || cache.calls != 2 {
		t.Fatalf("retry cache invalidation calls=%d err=%v", cache.calls, err)
	}
}

type orderingTransactionRunner struct{ inside bool }

func (runner *orderingTransactionRunner) Run(ctx context.Context, operation func(context.Context) error) error {
	runner.inside = true
	err := operation(ctx)
	runner.inside = false
	return err
}

type cacheInvalidatorFake struct {
	runner                  *orderingTransactionRunner
	err                     error
	calls                   int
	calledInsideTransaction bool
}

func (fake *cacheInvalidatorFake) Invalidate(_ context.Context, _ []uint) error {
	fake.calls++
	fake.calledInsideTransaction = fake.calledInsideTransaction || fake.runner.inside
	return fake.err
}

type cacheMetricsFake struct{ failures int }

func (fake *cacheMetricsFake) RecordCacheInvalidationFailure() { fake.failures++ }

type navigationAuthorizationFake struct {
	allUsers        []uint
	access          map[uint]navigation.UserAccess
	roleUsers       map[uint][]uint
	roles           map[uint]bool
	incremented     [][]uint
	incrementErr    error
	permissions     map[string]navigation.PermissionRef
	permissionRoles map[uint][]uint
}

func (fake *navigationAuthorizationFake) RoleExists(_ context.Context, roleID uint) (bool, error) {
	return fake.roles[roleID], nil
}
func (fake *navigationAuthorizationFake) UserAccess(_ context.Context, userID uint) (navigation.UserAccess, error) {
	access, exists := fake.access[userID]
	if !exists {
		return navigation.UserAccess{}, errors.New("user access not configured")
	}
	return access, nil
}
func (fake *navigationAuthorizationFake) UserIDsByRoleIDs(_ context.Context, roleIDs []uint) ([]uint, error) {
	result := make([]uint, 0)
	for _, roleID := range roleIDs {
		result = append(result, fake.roleUsers[roleID]...)
	}
	return result, nil
}
func (fake *navigationAuthorizationFake) AllUserIDs(context.Context) ([]uint, error) {
	return append([]uint(nil), fake.allUsers...), nil
}
func (fake *navigationAuthorizationFake) IncrementAccessVersions(_ context.Context, userIDs []uint) error {
	fake.incremented = append(fake.incremented, append([]uint(nil), userIDs...))
	return fake.incrementErr
}

func (fake *navigationAuthorizationFake) RoleIDsByPermissionCode(_ context.Context, code string) ([]uint, error) {
	permission, exists := fake.permissions[code]
	if !exists {
		return []uint{}, nil
	}
	return append([]uint(nil), fake.permissionRoles[permission.ID]...), nil
}
func (fake *navigationAuthorizationFake) LockPermission(_ context.Context, code string) (navigation.PermissionRef, bool, error) {
	permission, exists := fake.permissions[code]
	return permission, exists, nil
}
func (fake *navigationAuthorizationFake) EnsurePermission(_ context.Context, seed navigation.PermissionSeed) (navigation.PermissionRef, bool, error) {
	if permission, exists := fake.permissions[seed.Code]; exists {
		return permission, false, nil
	}
	permission := navigation.PermissionRef{ID: uint(len(fake.permissions) + 1), Code: seed.Code}
	fake.permissions[seed.Code] = permission
	return permission, true, nil
}
func (fake *navigationAuthorizationFake) MergePermissionRoles(_ context.Context, fromPermissionID, toPermissionID uint) error {
	fake.permissionRoles[toPermissionID] = uniqueTestIDs(append(fake.permissionRoles[toPermissionID], fake.permissionRoles[fromPermissionID]...))
	delete(fake.permissionRoles, fromPermissionID)
	return nil
}
func (fake *navigationAuthorizationFake) PermissionHasRoleReferences(_ context.Context, permissionID uint) (bool, error) {
	return len(fake.permissionRoles[permissionID]) > 0, nil
}
func (fake *navigationAuthorizationFake) DeletePermission(_ context.Context, permissionID uint) error {
	for code, permission := range fake.permissions {
		if permission.ID == permissionID {
			delete(fake.permissions, code)
		}
	}
	return nil
}

type navigationAPIFake struct {
	records map[uint]navigation.APIRecord
}

func (fake navigationAPIFake) ListByIDs(_ context.Context, ids []uint) ([]navigation.APIRecord, error) {
	result := make([]navigation.APIRecord, 0, len(ids))
	for _, id := range ids {
		if record, exists := fake.records[id]; exists {
			result = append(result, record)
		}
	}
	return result, nil
}

func (fake *navigationAPIFake) Lock(ctx context.Context, id uint) (navigation.APIRecord, error) {
	apis, err := fake.ListByIDs(ctx, []uint{id})
	if err != nil || len(apis) == 0 {
		return navigation.APIRecord{}, errors.New("API不存在")
	}
	return apis[0], nil
}
func (fake *navigationAPIFake) LockMany(ctx context.Context, ids []uint) ([]navigation.APIRecord, error) {
	return fake.ListByIDs(ctx, ids)
}
func (fake *navigationAPIFake) SetPermissionCode(_ context.Context, ids []uint, code string) error {
	for _, id := range ids {
		record, exists := fake.records[id]
		if !exists {
			return errors.New("API不存在")
		}
		record.PermissionCode = code
		fake.records[id] = record
	}
	return nil
}
func (fake *navigationAPIFake) CountPermissionCode(_ context.Context, code string) (int64, error) {
	var count int64
	for _, record := range fake.records {
		if record.PermissionCode == code {
			count++
		}
	}
	return count, nil
}

func uniqueTestIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
func newNavigationServiceFixture(t *testing.T) (*navigation.Service, *navigationAuthorizationFake) {
	t.Helper()
	service, authorization, _ := newNavigationServiceFixtureWithAPIs(t)
	return service, authorization
}

func newNavigationServiceFixtureWithAPIs(t *testing.T) (*navigation.Service, *navigationAuthorizationFake, *navigationAPIFake) {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(navigation.Models()...); err != nil {
		t.Fatalf("migrate navigation: %v", err)
	}
	authorization := &navigationAuthorizationFake{
		allUsers: []uint{10, 20}, access: make(map[uint]navigation.UserAccess),
		roleUsers: make(map[uint][]uint), roles: make(map[uint]bool),
		permissions: make(map[string]navigation.PermissionRef), permissionRoles: make(map[uint][]uint),
	}
	apis := &navigationAPIFake{records: make(map[uint]navigation.APIRecord)}
	service := navigation.NewService(navigation.NewGORMRepository(db), platformdatabase.NewTransactionRunner(db), authorization, apis)
	return service, authorization, apis
}
