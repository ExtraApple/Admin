package navigation_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	apiapplication "admin/internal/apimetadata/application"
	apidomain "admin/internal/apimetadata/domain"
	"admin/internal/navigation"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
)

func TestServiceCreatesAndReturnsOrderedMenuTree(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()

	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Settings", Sort: 2, Type: 1})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{ParentID: parent.ID, Name: "Users", Path: "/settings/users", Sort: 1, Type: 2})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Duplicate", Path: "/settings/users", Type: 2}); err == nil {
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

func TestServiceCreateMenuPreservesStatusPresenceInStorageAndVisibility(t *testing.T) {
	disabled, enabled, aboveRange := 0, 1, 2
	for _, test := range []struct {
		name      string
		status    *int
		want      int
		wantError bool
	}{
		{name: "omitted", want: 1},
		{name: "disabled", status: &disabled, want: 0},
		{name: "enabled", status: &enabled, want: 1},
		{name: "above range", status: &aboveRange, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, authorization := newNavigationServiceFixture(t)
			ctx := context.Background()
			menu, err := service.CreateMenu(ctx, navigation.CreateInput{
				Name: "Status menu", Path: "/status-menu", Type: 2,
				PermissionCode: "status.read", Status: test.status,
			})
			if test.wantError {
				if code, ok := navigation.CodeOf(err); !ok || code != navigation.CodeValidationInvalid {
					t.Fatalf("invalid status error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("create menu: %v", err)
			}
			if menu.ID == 0 || menu.Status != test.want {
				t.Fatalf("created menu = %#v, want nonzero ID and status %d", menu, test.want)
			}
			tree, err := service.MenuTree(ctx)
			if err != nil {
				t.Fatalf("read persisted menu tree: %v", err)
			}
			if len(tree) != 1 || tree[0].ID != menu.ID || tree[0].Status != test.want {
				t.Fatalf("persisted tree = %#v, want menu %d with status %d", tree, menu.ID, test.want)
			}
			authorization.roles[7] = true
			authorization.access[42] = navigation.UserAccess{RoleIDs: []uint{7}, Permissions: []string{"status.read"}}
			if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID}); err != nil {
				t.Fatalf("assign menu: %v", err)
			}
			visible, err := service.UserMenus(ctx, 42)
			if err != nil {
				t.Fatalf("read user menus: %v", err)
			}
			if test.want == 0 {
				if len(visible) != 0 {
					t.Fatalf("disabled menu is visible: %#v", visible)
				}
			} else if len(visible) != 1 || visible[0].ID != menu.ID || visible[0].Status != 1 {
				t.Fatalf("enabled menu is not visible: %#v", visible)
			}
		})
	}
}
func TestServiceUpdatesAndDeletesOnlyLeafMenus(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Parent", Path: "/parent", Type: 2})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{ParentID: parent.ID, Name: "Child", Path: "/child", Type: 2})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := service.UpdateMenu(ctx, child.ID, navigation.UpdateInput{ParentID: &child.ID}); err == nil {
		t.Fatal("self parent was accepted")
	}
	if _, err := service.UpdateMenu(ctx, child.ID, navigation.UpdateInput{Path: stringPointer("/parent")}); err == nil {
		t.Fatal("duplicate update path was accepted")
	}
	sort := 7
	updated, err := service.UpdateMenu(ctx, child.ID, navigation.UpdateInput{Name: stringPointer("Renamed"), Sort: &sort})
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

func TestServiceResolvesExplicitEmptyCodeFromBoundAPIs(t *testing.T) {
	for _, test := range []struct {
		name     string
		apiCodes []string
		menuCode string
		wantCode string
		wantErr  bool
	}{
		{name: "existing API code", apiCodes: []string{"users.read"}, menuCode: "users.read", wantCode: "users.read"},
		{name: "derive uncoded API", apiCodes: []string{""}, wantCode: "admin.users.get"},
		{name: "conflicting API codes", apiCodes: []string{"users.read", "groups.read"}, menuCode: "actions.read", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			db := testutil.OpenIsolatedSQLite(t)
			models := append(navigation.Models(), apigorm.Models()...)
			if err := db.AutoMigrate(models...); err != nil {
				t.Fatalf("migrate menu update fixture: %v", err)
			}
			repository := navigation.NewGORMRepository(db)
			authorization := &navigationAuthorizationFake{
				allUsers: []uint{42}, access: make(map[uint]navigation.UserAccess),
				roleUsers: make(map[uint][]uint), roles: make(map[uint]bool),
				permissions: make(map[string]navigation.PermissionRef), permissionRoles: make(map[uint][]uint),
			}
			apiCore := apiapplication.NewCore(apigorm.NewRepository(db), platformdatabase.NewTransactionRunner(db), authorization)
			service := navigation.NewService(repository, platformdatabase.NewTransactionRunner(db), authorization, navigationAPICoreStorage{core: apiCore})
			menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Actions", PermissionCode: test.menuCode, Sort: 1, Type: 3})
			if err != nil {
				t.Fatalf("create menu: %v", err)
			}
			apiIDs := make([]uint, 0, len(test.apiCodes))
			paths := []string{"/api/admin/users", "/api/admin/groups"}
			for index, code := range test.apiCodes {
				api, err := apiCore.Create(ctx, apiapplication.CreateInput{
					Name: "Read", Method: "GET", Path: paths[index], PermissionCode: code,
				})
				if err != nil {
					t.Fatalf("create API metadata: %v", err)
				}
				apiIDs = append(apiIDs, api.ID)
			}
			if err := repository.ReplaceMenuAPIs(ctx, menu.ID, apiIDs); err != nil {
				t.Fatalf("prepare existing direct bindings: %v", err)
			}

			empty, disabled, sort := " \t ", 0, 5
			updated, err := service.UpdateMenu(ctx, menu.ID, navigation.UpdateInput{Name: stringPointer("Renamed"), PermissionCode: &empty, Status: &disabled, Sort: &sort})
			if test.wantErr {
				if code, ok := navigation.CodeOf(err); !ok || code != navigation.CodeValidationInvalid {
					t.Fatalf("conflicting empty-code update error = %v, want Navigation validation error", err)
				}
			} else if err != nil || updated.ID != menu.ID || updated.PermissionCode != test.wantCode || updated.Name != "Renamed" || updated.Status != disabled || updated.Sort != sort {
				t.Fatalf("resolved update = %#v, error = %v", updated, err)
			}
			tree, err := service.MenuTree(ctx)
			if err != nil || len(tree) != 1 {
				t.Fatalf("persisted menu tree = %#v, error = %v", tree, err)
			}
			if test.wantErr {
				if tree[0].PermissionCode != menu.PermissionCode || tree[0].Name != menu.Name || tree[0].Status != menu.Status || tree[0].Sort != menu.Sort {
					t.Fatalf("conflict left menu changes: %#v, before = %#v", tree[0], menu)
				}
			} else if tree[0].PermissionCode != updated.PermissionCode || tree[0].Name != updated.Name || tree[0].Status != updated.Status || tree[0].Sort != updated.Sort {
				t.Fatalf("persisted menu = %#v, response = %#v", tree[0], updated)
			}
			bound, err := service.MenuAPIs(ctx, menu.ID)
			if err != nil || len(bound) != len(apiIDs) {
				t.Fatalf("direct bindings = %#v, error = %v", bound, err)
			}
			for index, apiID := range apiIDs {
				wantCode := test.wantCode
				if test.wantErr {
					wantCode = test.apiCodes[index]
				}
				found := false
				for _, api := range bound {
					if api.ID == apiID {
						found = true
						if api.PermissionCode != wantCode {
							t.Fatalf("bound API %d code = %q, want %q", apiID, api.PermissionCode, wantCode)
						}
					}
				}
				if !found {
					t.Fatalf("direct binding %d was lost: %#v", apiID, bound)
				}
			}
		})
	}
}

func TestServicePreservesUnboundCodeUpdateAndOmittedCodeSemantics(t *testing.T) {
	service, _, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Actions", PermissionCode: "actions.original", Type: 3})
	if err != nil {
		t.Fatalf("create menu: %v", err)
	}
	newCode := " actions.updated "
	updated, err := service.UpdateMenu(ctx, menu.ID, navigation.UpdateInput{PermissionCode: &newCode})
	if err != nil || updated.PermissionCode != "actions.updated" {
		t.Fatalf("unbound code update = %#v, error = %v", updated, err)
	}
	empty := " \t "
	updated, err = service.UpdateMenu(ctx, menu.ID, navigation.UpdateInput{PermissionCode: &empty})
	if err != nil || updated.PermissionCode != "" {
		t.Fatalf("unbound explicit empty code = %#v, error = %v", updated, err)
	}
	bound, err := service.MenuAPIs(ctx, menu.ID)
	if err != nil || len(bound) != 0 {
		t.Fatalf("unbound code updates created bindings: %#v, error = %v", bound, err)
	}

	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Read Users", Method: "GET", Path: "/api/admin/users", Status: 1, NeedAuth: 1}
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "users.read"); err != nil {
		t.Fatalf("prepare direct binding: %v", err)
	}
	disabled := 0
	updated, err = service.UpdateMenu(ctx, menu.ID, navigation.UpdateInput{Name: stringPointer("Renamed"), Status: &disabled})
	if err != nil || updated.Name != "Renamed" || updated.Status != disabled || updated.PermissionCode != "users.read" {
		t.Fatalf("ordinary bound update = %#v, error = %v", updated, err)
	}
	tree, err := service.MenuTree(ctx)
	if err != nil || len(tree) != 1 || tree[0].Name != updated.Name || tree[0].Status != updated.Status || tree[0].PermissionCode != updated.PermissionCode {
		t.Fatalf("persisted ordinary update = %#v, error = %v", tree, err)
	}
	bound, err = service.MenuAPIs(ctx, menu.ID)
	if err != nil || len(bound) != 1 || bound[0].ID != 9 || bound[0].PermissionCode != "users.read" {
		t.Fatalf("omitted code changed direct binding: %#v, error = %v", bound, err)
	}
}
func TestServiceAssignsRoleMenusAndPreservesVisibleParentChain(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()
	root, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Root", Path: "/root", PermissionCode: "root.hidden", Type: 2})
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
		{Name: "Admin", Path: "/admin", Sort: 1, Type: 2},
		{Name: "Users", Path: "/admin/users", ParentPath: "/admin", Component: "views/users", Sort: 2, Type: 2},
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

func TestServiceRejectsMixedZeroAPIIDsWithoutChangingExistingBinding(t *testing.T) {
	service, _, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Existing Actions", Type: 3})
	if err != nil {
		t.Fatal(err)
	}
	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Read Users", Method: "GET", Path: "/api/admin/users", Status: 1, NeedAuth: 1}
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "users.original"); err != nil {
		t.Fatalf("prepare existing binding: %v", err)
	}

	err = service.AssignAPIs(ctx, menu.ID, []uint{9, 0}, "users.must-not-persist")
	if code, ok := navigation.CodeOf(err); !ok || code != navigation.CodeValidationInvalid {
		t.Fatalf("mixed zero IDs error = %v, want Navigation validation error", err)
	}
	tree, err := service.MenuTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].ID != menu.ID || tree[0].PermissionCode != "users.original" {
		t.Fatalf("rejected IDs changed menu facts: %#v", tree)
	}
	bound, err := service.MenuAPIs(ctx, menu.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound) != 1 || bound[0].ID != 9 || bound[0].PermissionCode != "users.original" {
		t.Fatalf("rejected IDs changed API binding: %#v", bound)
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
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Parent", Type: 1})
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
func TestServiceDeleteAPIHardDeletesMetadataAndRollsBackLinkedWrites(t *testing.T) {
	t.Run("commits relation cleanup and hard delete", func(t *testing.T) {
		fixture := newAPIDeleteFixture(t)

		err := fixture.service.DeleteAPI(fixture.ctx, fixture.apiID, func(transactionContext context.Context) error {
			return fixture.apiCore.Delete(transactionContext, fixture.apiID)
		})
		if err != nil {
			t.Fatalf("DeleteAPI() error = %v", err)
		}
		fixture.assertDeleted(t)
		if len(fixture.authorization.incremented) != 1 || len(fixture.authorization.incremented[0]) != 1 || fixture.authorization.incremented[0][0] != 42 {
			t.Fatalf("affected user invalidations = %#v, want [[42]]", fixture.authorization.incremented)
		}
	})

	t.Run("rolls back relation cleanup when delete callback fails", func(t *testing.T) {
		fixture := newAPIDeleteFixture(t)
		deleteErr := errors.New("delete API failed")

		err := fixture.service.DeleteAPI(fixture.ctx, fixture.apiID, func(context.Context) error {
			return deleteErr
		})
		if !errors.Is(err, deleteErr) {
			t.Fatalf("DeleteAPI() error = %v, want %v", err, deleteErr)
		}
		fixture.assertRetained(t)
	})

	t.Run("rolls back hard delete when access version update fails", func(t *testing.T) {
		fixture := newAPIDeleteFixture(t)
		versionErr := errors.New("access version update failed")
		fixture.authorization.incrementErr = versionErr

		err := fixture.service.DeleteAPI(fixture.ctx, fixture.apiID, func(transactionContext context.Context) error {
			return fixture.apiCore.Delete(transactionContext, fixture.apiID)
		})
		if !errors.Is(err, versionErr) {
			t.Fatalf("DeleteAPI() error = %v, want %v", err, versionErr)
		}
		fixture.assertRetained(t)
	})
}

type apiDeleteFixture struct {
	ctx           context.Context
	db            *gorm.DB
	service       *navigation.Service
	apiCore       *apiapplication.Core
	authorization *navigationAuthorizationFake
	apiID         uint
	menuID        uint
}

func newAPIDeleteFixture(t *testing.T) apiDeleteFixture {
	t.Helper()
	ctx := context.Background()
	db := testutil.OpenIsolatedSQLite(t)
	models := append(navigation.Models(), apigorm.Models()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate API delete fixture: %v", err)
	}
	authorization := &navigationAuthorizationFake{
		allUsers: []uint{42}, access: make(map[uint]navigation.UserAccess),
		roleUsers: map[uint][]uint{7: {42}}, roles: map[uint]bool{7: true},
		permissions: make(map[string]navigation.PermissionRef), permissionRoles: make(map[uint][]uint),
	}
	apiCore := apiapplication.NewCore(apigorm.NewRepository(db), platformdatabase.NewTransactionRunner(db), authorization)
	api, err := apiCore.Create(ctx, apiapplication.CreateInput{Name: "Delete User", Method: "DELETE", Path: "/api/admin/users/:id"})
	if err != nil {
		t.Fatalf("create API metadata: %v", err)
	}
	service := navigation.NewService(
		navigation.NewGORMRepository(db),
		platformdatabase.NewTransactionRunner(db),
		authorization,
		navigationAPICoreStorage{core: apiCore},
	)
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Delete User", Type: 3})
	if err != nil {
		t.Fatalf("create menu: %v", err)
	}
	if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID}); err != nil {
		t.Fatalf("assign role menu: %v", err)
	}
	if err := service.AssignAPIs(ctx, menu.ID, []uint{api.ID}, "users.delete"); err != nil {
		t.Fatalf("assign API: %v", err)
	}
	authorization.incremented = nil
	return apiDeleteFixture{ctx: ctx, db: db, service: service, apiCore: apiCore, authorization: authorization, apiID: api.ID, menuID: menu.ID}
}

func (fixture apiDeleteFixture) assertDeleted(t *testing.T) {
	t.Helper()
	if _, err := fixture.apiCore.Get(fixture.ctx, fixture.apiID); err == nil {
		t.Fatal("hard-deleted API remained queryable")
	}
	var count int64
	if err := fixture.db.Unscoped().Table("apis").Where("id = ?", fixture.apiID).Count(&count).Error; err != nil {
		t.Fatalf("count unscoped API rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("unscoped API rows = %d, want 0", count)
	}
	bound, err := fixture.service.MenuAPIs(fixture.ctx, fixture.menuID)
	if err != nil || len(bound) != 0 {
		t.Fatalf("menu API bindings after delete = %#v, error = %v", bound, err)
	}
}

func (fixture apiDeleteFixture) assertRetained(t *testing.T) {
	t.Helper()
	if _, err := fixture.apiCore.Get(fixture.ctx, fixture.apiID); err != nil {
		t.Fatalf("API metadata was not rolled back: %v", err)
	}
	bound, err := fixture.service.MenuAPIs(fixture.ctx, fixture.menuID)
	if err != nil || len(bound) != 1 || bound[0].ID != fixture.apiID {
		t.Fatalf("menu API bindings after rollback = %#v, error = %v", bound, err)
	}
}

type navigationAPICoreStorage struct{ core *apiapplication.Core }

func (storage navigationAPICoreStorage) ListByIDs(ctx context.Context, ids []uint) ([]navigation.APIRecord, error) {
	apis, err := storage.core.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]navigation.APIRecord, len(apis))
	for index := range apis {
		result[index] = navigationAPIRecord(apis[index])
	}
	return result, nil
}

func (storage navigationAPICoreStorage) ListByPermissionCode(ctx context.Context, code string) ([]navigation.APIRecord, error) {
	apis, err := storage.core.ListByPermissionCode(ctx, code)
	if err != nil {
		return nil, err
	}
	result := make([]navigation.APIRecord, len(apis))
	for index := range apis {
		result[index] = navigationAPIRecord(apis[index])
	}
	return result, nil
}

func (storage navigationAPICoreStorage) LockDependencies(ctx context.Context, ids []uint, codes []string) ([]navigation.APIRecord, error) {
	apis, err := storage.core.LockDependencies(ctx, ids, codes)
	if err != nil {
		return nil, err
	}
	result := make([]navigation.APIRecord, len(apis))
	for index := range apis {
		result[index] = navigationAPIRecord(apis[index])
	}
	return result, nil
}

func (storage navigationAPICoreStorage) Lock(ctx context.Context, id uint) (navigation.APIRecord, error) {
	api, err := storage.core.Lock(ctx, id)
	return navigationAPIRecord(api), err
}

func (storage navigationAPICoreStorage) LockMany(ctx context.Context, ids []uint) ([]navigation.APIRecord, error) {
	apis, err := storage.core.LockMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]navigation.APIRecord, len(apis))
	for index := range apis {
		result[index] = navigationAPIRecord(apis[index])
	}
	return result, nil
}

func (storage navigationAPICoreStorage) SetPermissionCode(ctx context.Context, ids []uint, code string) error {
	return storage.core.SetPermissionCode(ctx, ids, code)
}

func (storage navigationAPICoreStorage) CountPermissionCode(ctx context.Context, code string) (int64, error) {
	return storage.core.CountPermissionCode(ctx, code)
}

func navigationAPIRecord(api apidomain.API) navigation.APIRecord {
	return navigation.APIRecord{
		ID: api.ID, Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group,
		PermissionCode: api.PermissionCode, Sort: api.Sort, Status: api.Status,
		NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit, Remark: api.Remark, Deleted: api.Deleted,
	}
}
func TestServiceGeneratesButtonMenuFromAuthenticatedAPI(t *testing.T) {
	service, authorization, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Users", Path: "/users", Type: 2})
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
func TestGenerateMenuButtonEnforcesParentAndAPIPolicy(t *testing.T) {
	service, _, apis := newNavigationServiceFixtureWithAPIs(t)
	ctx := context.Background()
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Pages", Path: "/pages", Type: 2})
	if err != nil {
		t.Fatalf("create page parent: %v", err)
	}
	buttonParent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Button parent", Type: 3})
	if err != nil {
		t.Fatalf("create button parent: %v", err)
	}
	apis.records[10] = navigation.APIRecord{ID: 10, Name: "Public", Method: "GET", Path: "/api/public", Status: 1, NeedAuth: 0}
	if _, err := service.GenerateMenuButton(ctx, 10, parent.ID, "Public", 1); err == nil {
		t.Fatal("public API generated a button")
	}
	apis.records[11] = navigation.APIRecord{ID: 11, Name: "Stopped", Method: "GET", Path: "/api/stopped", Status: 0, NeedAuth: 1}
	if _, err := service.GenerateMenuButton(ctx, 11, parent.ID, "Stopped", 1); err == nil {
		t.Fatal("stopped API generated a button")
	}
	apis.records[9] = navigation.APIRecord{ID: 9, Name: "Authenticated", Method: "GET", Path: "/api/authenticated", Status: 1, NeedAuth: 1}
	if _, err := service.GenerateMenuButton(ctx, 9, buttonParent.ID, "Nested", 1); err == nil {
		t.Fatal("button generated under a button parent")
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
func TestServiceClearAPIsOnlyInvalidatesOnChange(t *testing.T) {
	fixture := newAPIDeleteFixture(t)
	fixture.authorization.incremented = nil

	if err := fixture.service.ClearAPIs(fixture.ctx, fixture.menuID); err != nil {
		t.Fatalf("clear APIs: %v", err)
	}
	if len(fixture.authorization.incremented) != 1 || len(fixture.authorization.incremented[0]) != 1 || fixture.authorization.incremented[0][0] != 42 {
		t.Fatalf("clear access-version invalidations = %#v, want [[42]]", fixture.authorization.incremented)
	}
	bound, err := fixture.service.MenuAPIs(fixture.ctx, fixture.menuID)
	if err != nil || len(bound) != 0 {
		t.Fatalf("bindings after clear = %#v, error = %v", bound, err)
	}

	fixture.authorization.incremented = nil
	if err := fixture.service.ClearAPIs(fixture.ctx, fixture.menuID); err != nil {
		t.Fatalf("clear already-empty APIs: %v", err)
	}
	if len(fixture.authorization.incremented) != 0 {
		t.Fatalf("no-op clear invalidated access versions: %#v", fixture.authorization.incremented)
	}
}
func TestServiceClearAPIsRollsBackOnVersionFailure(t *testing.T) {
	fixture := newAPIDeleteFixture(t)
	fixture.authorization.incrementErr = errors.New("version failure")
	if err := fixture.service.ClearAPIs(fixture.ctx, fixture.menuID); err == nil {
		t.Fatal("clear APIs succeeded despite version failure")
	}
	bound, err := fixture.service.MenuAPIs(fixture.ctx, fixture.menuID)
	if err != nil || len(bound) != 1 || bound[0].ID != fixture.apiID {
		t.Fatalf("clear rollback bindings=%#v error=%v", bound, err)
	}
}

type orderingTransactionRunner struct{ inside bool }

func (runner *orderingTransactionRunner) Run(ctx context.Context, operation func(context.Context) error) error {
	runner.inside = true
	err := operation(ctx)
	runner.inside = false
	return err
}

func (runner *orderingTransactionRunner) AfterCommit(_ context.Context, callback func()) {
	callback()
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
	protectedRoles  map[uint]bool
	incremented     [][]uint
	incrementErr    error
	permissions     map[string]navigation.PermissionRef
	permissionRoles map[uint][]uint
}

func (fake *navigationAuthorizationFake) RoleConfiguration(_ context.Context, roleID uint) (navigation.RoleConfigurationFact, error) {
	return navigation.RoleConfigurationFact{Exists: fake.roles[roleID], Protected: fake.protectedRoles[roleID]}, nil
}
func (fake *navigationAuthorizationFake) LockRoleConfiguration(ctx context.Context, roleID uint) (navigation.RoleConfigurationFact, error) {
	return fake.RoleConfiguration(ctx, roleID)
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

func (fake *navigationAuthorizationFake) LockAccessVersions(context.Context, []uint) error {
	return nil
}

func (fake *navigationAuthorizationFake) ExistingPermissionCodes(_ context.Context, codes []string, _ bool) ([]string, error) {
	existing := make([]string, 0, len(codes))
	for _, code := range codes {
		if _, exists := fake.permissions[code]; exists {
			existing = append(existing, code)
		}
	}
	return existing, nil
}

func (fake *navigationAuthorizationFake) RoleIDsByPermissionCode(_ context.Context, code string) ([]uint, error) {
	permission, exists := fake.permissions[code]
	if !exists {
		return []uint{}, nil
	}
	return append([]uint(nil), fake.permissionRoles[permission.ID]...), nil
}
func (fake *navigationAuthorizationFake) LockPermissions(context.Context, []string) error {
	return nil
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
		if record, exists := fake.records[id]; exists && !record.Deleted {
			result = append(result, record)
		}
	}
	return result, nil
}

func (fake navigationAPIFake) ListByPermissionCode(ctx context.Context, code string) ([]navigation.APIRecord, error) {
	return fake.LockDependencies(ctx, nil, []string{code})
}

func (fake navigationAPIFake) LockDependencies(_ context.Context, ids []uint, codes []string) ([]navigation.APIRecord, error) {
	selected := make(map[uint]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	for id, record := range fake.records {
		for _, code := range codes {
			if record.PermissionCode == code {
				selected[id] = true
				break
			}
		}
	}
	ordered := make([]uint, 0, len(selected))
	for id := range selected {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	records := make([]navigation.APIRecord, 0, len(ordered))
	for _, id := range ordered {
		if record, exists := fake.records[id]; exists {
			records = append(records, record)
		}
	}
	return records, nil
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

func TestRoleMenusReturnsDisabledAndOrphanAssignmentsWithoutChangingUserMenus(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()
	root, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Unassigned parent", Path: "/unassigned", Sort: 5, Type: 2})
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Assigned child", Path: "/unassigned/child", ParentID: root.ID, Sort: 2, PermissionCode: "child.read", Type: 2})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Disabled assigned", Path: "/disabled", Sort: 1, PermissionCode: "disabled.read", Type: 2})
	if err != nil {
		t.Fatal(err)
	}
	off := 0
	if _, err := service.UpdateMenu(ctx, disabled.ID, navigation.UpdateInput{Status: &off}); err != nil {
		t.Fatal(err)
	}
	authorization.roles[7] = true
	authorization.access[42] = navigation.UserAccess{RoleIDs: []uint{7}, Permissions: []string{"child.read", "disabled.read"}}
	if err := service.AssignRoleMenus(ctx, 7, []uint{child.ID, disabled.ID}); err != nil {
		t.Fatal(err)
	}
	menus, err := service.RoleMenus(ctx, 7)
	if err != nil || len(menus) != 2 || menus[0].ID != disabled.ID || menus[0].Status != 0 || menus[1].ID != child.ID || menus[1].ParentID != root.ID || len(menus[1].Children) != 0 {
		t.Fatalf("flat assigned menus = %#v, %v", menus, err)
	}
	userTree, err := service.UserMenus(ctx, 42)
	if err != nil || len(userTree) != 1 || userTree[0].ID != root.ID || len(userTree[0].Children) != 1 || userTree[0].Children[0].ID != child.ID {
		t.Fatalf("user menus = %#v, %v", userTree, err)
	}
}

func TestRoleMenuReplacementRejectsIllegalReferencesAndProtectedRoleAtomically(t *testing.T) {
	for _, test := range []struct {
		name      string
		targets   func(uint, uint) []uint
		protected bool
		code      navigation.ErrorCode
	}{
		{"raw zero", func(old, next uint) []uint { return []uint{next, 0} }, false, navigation.CodeValidationInvalid},
		{"missing menu", func(old, next uint) []uint { return []uint{next, 999} }, false, navigation.CodeValidationInvalid},
		{"protected role clear", func(old, next uint) []uint { return []uint{} }, true, navigation.CodeConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, authorization := newNavigationServiceFixture(t)
			ctx := context.Background()
			old, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Existing", Path: "/existing", Type: 2})
			if err != nil {
				t.Fatal(err)
			}
			next, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Replacement", Path: "/replacement", Type: 2})
			if err != nil {
				t.Fatal(err)
			}
			authorization.roles[7] = true
			if err := service.AssignRoleMenus(ctx, 7, []uint{old.ID}); err != nil {
				t.Fatal(err)
			}
			authorization.protectedRoles = map[uint]bool{7: test.protected}
			err = service.AssignRoleMenus(ctx, 7, test.targets(old.ID, next.ID))
			if code, _ := navigation.CodeOf(err); code != test.code {
				t.Fatalf("replacement error = %v, want %s", err, test.code)
			}
			stored, err := service.RoleMenus(ctx, 7)
			if err != nil || len(stored) != 1 || stored[0].ID != old.ID {
				t.Fatalf("rejected replacement changed role menus = %#v, %v", stored, err)
			}
		})
	}
}

func TestMenuLifecycleRejectsDeletedParentsAndMenuReferences(t *testing.T) {
	fixture := newAPIDeleteFixture(t)
	ctx, service := fixture.ctx, fixture.service
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Parent", Path: "/lifecycle-parent", Type: 2})
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Child", ParentID: parent.ID, Type: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteMenu(ctx, parent.ID); err == nil {
		t.Fatal("parent deletion bypassed the committed child")
	}
	if err := service.DeleteMenu(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteMenu(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	fixture.authorization.incremented = nil
	if _, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Stale child", ParentID: parent.ID, Type: 1}); err == nil {
		t.Fatal("created a child of a deleted parent")
	}
	if _, err := service.UpdateMenu(ctx, fixture.menuID, navigation.UpdateInput{Name: stringPointer("Must roll back"), ParentID: &parent.ID}); err == nil {
		t.Fatal("moved a menu under a deleted parent")
	}
	if _, err := service.GenerateMenuButton(ctx, fixture.apiID, parent.ID, "Stale button", 0); err == nil {
		t.Fatal("generated a button under a deleted parent")
	}
	if err := service.AssignRoleMenus(ctx, 7, []uint{fixture.menuID, child.ID}); err == nil {
		t.Fatal("bound a role to a deleted menu")
	}
	assigned, err := service.RoleMenus(ctx, 7)
	if err != nil || len(assigned) != 1 || assigned[0].ID != fixture.menuID || assigned[0].Name != "Delete User" {
		t.Fatalf("rejected writes changed existing menu configuration: %#v, %v", assigned, err)
	}
	var count int64
	if err := fixture.db.Table("menus").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("rejected parent references left menu rows: %d, %v", count, err)
	}
	if len(fixture.authorization.incremented) != 0 {
		t.Fatalf("rejected references invalidated users: %#v", fixture.authorization.incremented)
	}
}

func TestAPILifecycleStopPreservesConfigurationButRefusesNewBindings(t *testing.T) {
	fixture := newAPIDeleteFixture(t)
	ctx := fixture.ctx
	apiService := apiapplication.NewService(fixture.apiCore, nil)
	disabled := 0
	stopped, err := apiService.Update(ctx, fixture.apiID, apiapplication.UpdateInput{Status: &disabled})
	if err != nil || stopped.Status != 0 {
		t.Fatalf("stop API: %#v, %v", stopped, err)
	}
	policy, err := fixture.apiCore.Policy(ctx, stopped.Method, stopped.Path)
	if err != nil || policy.Status != 0 {
		t.Fatalf("runtime policy still enables the stopped API: %#v, %v", policy, err)
	}
	bound, err := fixture.service.MenuAPIs(ctx, fixture.menuID)
	if err != nil || len(bound) != 1 || bound[0].ID != fixture.apiID || bound[0].Status != 0 {
		t.Fatalf("legal stop removed configuration: %#v, %v", bound, err)
	}
	target, err := fixture.service.CreateMenu(ctx, navigation.CreateInput{Name: "New binding", Type: 3})
	if err != nil {
		t.Fatal(err)
	}
	fixture.authorization.incremented = nil
	if err := fixture.service.AssignAPIs(ctx, target.ID, []uint{fixture.apiID}, "users.delete"); err == nil {
		t.Fatal("new binding accepted a stopped API")
	}
	if _, err := fixture.service.GenerateMenuButton(ctx, fixture.apiID, target.ID, "Stopped button", 0); err == nil {
		t.Fatal("button generation accepted a stopped API")
	}
	bound, err = fixture.service.MenuAPIs(ctx, target.ID)
	if err != nil || len(bound) != 0 || len(fixture.authorization.incremented) != 0 {
		t.Fatalf("failed stopped-API binding changed facts: %#v, %v, versions=%#v", bound, err, fixture.authorization.incremented)
	}
	if err := fixture.service.DeleteAPI(ctx, fixture.apiID, func(transactionContext context.Context) error {
		return fixture.apiCore.Delete(transactionContext, fixture.apiID)
	}); err != nil {
		t.Fatal(err)
	}
	fixture.assertDeleted(t)
	fixture.authorization.incremented = nil
	if err := fixture.service.AssignAPIs(ctx, target.ID, []uint{fixture.apiID}, "users.delete"); err == nil {
		t.Fatal("new binding accepted deleted API metadata")
	}
	bound, err = fixture.service.MenuAPIs(ctx, target.ID)
	if err != nil || len(bound) != 0 || len(fixture.authorization.incremented) != 0 {
		t.Fatalf("deleted API was rebound: %#v, %v, versions=%#v", bound, err, fixture.authorization.incremented)
	}
}

func TestNestedNavigationCacheInvalidationWaitsForActualOuterCommit(t *testing.T) {
	fixture := newAPIDeleteFixture(t)
	runner := platformdatabase.NewTransactionRunner(fixture.db)
	cache := &navigationCacheRecorder{}
	service := navigation.NewService(navigation.NewGORMRepository(fixture.db), runner, fixture.authorization,
		navigationAPICoreStorage{core: fixture.apiCore}, navigation.WithCacheInvalidation(cache, nil))
	ctx := fixture.ctx
	rollback := errors.New("outer request failed")
	err := runner.Run(ctx, func(transactionContext context.Context) error {
		if err := service.AssignAPIs(transactionContext, fixture.menuID, []uint{fixture.apiID}, "rolled.back"); err != nil {
			return err
		}
		if len(cache.calls) != 0 {
			t.Fatal("cache invalidation escaped a still-open outer transaction")
		}
		return rollback
	})
	if !errors.Is(err, rollback) || len(cache.calls) != 0 {
		t.Fatalf("rolled-back outer request invalidated cache: %v, %#v", err, cache.calls)
	}
	bound, err := service.MenuAPIs(ctx, fixture.menuID)
	if err != nil || len(bound) != 1 || bound[0].PermissionCode != "users.delete" {
		t.Fatalf("outer rollback did not restore API configuration: %#v, %v", bound, err)
	}
	err = runner.Run(ctx, func(transactionContext context.Context) error {
		if err := service.AssignAPIs(transactionContext, fixture.menuID, []uint{fixture.apiID}, "committed.code"); err != nil {
			return err
		}
		if len(cache.calls) != 0 {
			t.Fatal("cache invalidation ran before the real outer commit")
		}
		return nil
	})
	if err != nil || len(cache.calls) != 1 || len(cache.calls[0]) != 1 || cache.calls[0][0] != 42 {
		t.Fatalf("outer commit cache invalidation: %v, %#v", err, cache.calls)
	}
	bound, err = service.MenuAPIs(ctx, fixture.menuID)
	if err != nil || len(bound) != 1 || bound[0].PermissionCode != "committed.code" {
		t.Fatalf("successful outer transaction did not commit API configuration: %#v, %v", bound, err)
	}
}

type navigationCacheRecorder struct{ calls [][]uint }

func (cache *navigationCacheRecorder) Invalidate(_ context.Context, userIDs []uint) error {
	cache.calls = append(cache.calls, append([]uint(nil), userIDs...))
	return nil
}

func stringPointer(value string) *string { return &value }
