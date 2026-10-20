package navigation_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"admin/internal/navigation"
	platformdatabase "admin/internal/platform/database"
)

func TestMenuNormalizedValuesAndRealDisplayChanges(t *testing.T) {
	fixture := newConfigurationNavigationFixture(t)
	service := fixture.service
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{
		Name: "Actions", Path: "/actions", Component: "Actions", Icon: "bolt", PermissionCode: "actions.read", Sort: 2, Type: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeVersions := configurationVersions(t, fixture.db)
	parent, sort, menuType, status := uint(0), 2, 3, 1
	updated, err := service.UpdateMenu(ctx, menu.ID, navigation.UpdateInput{
		ParentID: &parent, Name: stringPointer("  Actions  "), Path: stringPointer("  /actions  "),
		Component: stringPointer("Actions"), Icon: stringPointer("bolt"), PermissionCode: stringPointer(" actions.read "),
		Sort: &sort, Type: &menuType, Status: &status,
	})
	versions := configurationVersions(t, fixture.db)
	if err != nil || !reflect.DeepEqual(updated, menu) || !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("normalized menu=%#v error=%v versions=%v, want unchanged %#v", updated, err, versions, menu)
	}
	for _, update := range []navigation.UpdateInput{
		{Name: stringPointer("Renamed")}, {Icon: stringPointer("")}, {Sort: &status},
	} {
		if _, err := service.UpdateMenu(ctx, menu.ID, update); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := service.MenuTree(ctx)
	if err != nil || len(tree) != 1 || tree[0].Name != "Renamed" || tree[0].Icon != "" || tree[0].Sort != 1 {
		t.Fatalf("display changes did not persist: %#v, %v", tree, err)
	}
	versions = configurationVersions(t, fixture.db)
	want := map[uint]int{10: beforeVersions[10] + 3, 20: beforeVersions[20] + 3}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("display changes did not retain all-user invalidation: got=%v want=%v", versions, want)
	}
}

func TestEqualMenuFieldsDoNotBypassUpdateValidation(t *testing.T) {
	fixture := newConfigurationNavigationFixture(t)
	service := fixture.service
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Keep", Path: "/keep"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Other", Path: "/other"}); err != nil {
		t.Fatal(err)
	}
	before, err := service.MenuTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeVersions := configurationVersions(t, fixture.db)
	missing := uint(999)
	for _, input := range []navigation.UpdateInput{
		{}, {Name: stringPointer(" ")},
		{Name: stringPointer("Keep"), ParentID: &menu.ID},
		{Name: stringPointer("Keep"), ParentID: &missing},
		{Name: stringPointer("Keep"), Path: stringPointer("/other")},
	} {
		if _, err := service.UpdateMenu(ctx, menu.ID, input); err == nil {
			t.Fatalf("invalid update accepted: %#v", input)
		}
	}
	after, err := service.MenuTree(ctx)
	versions := configurationVersions(t, fixture.db)
	if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("invalid updates changed menus=%#v versions=%v error=%v", after, versions, err)
	}
}

func TestRoleMenusCompareDisabledConfigurationNotRuntimeVisibility(t *testing.T) {
	fixture := newConfigurationNavigationFixture(t)
	service, authorization := fixture.service, fixture.authorization
	ctx := context.Background()
	disabled := 0
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Hidden", PermissionCode: "hidden.read", Status: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	authorization.roles[7] = true
	authorization.roleUsers[7] = []uint{42}
	authorization.access[42] = navigation.UserAccess{RoleIDs: []uint{7}}
	if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID}); err != nil {
		t.Fatal(err)
	}
	beforeVersions := configurationVersions(t, fixture.db)
	if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID, menu.ID}); err != nil {
		t.Fatal(err)
	}
	configured, err := service.RoleMenus(ctx, 7)
	if err != nil || len(configured) != 1 || configured[0].ID != menu.ID || configured[0].Status != 0 {
		t.Fatalf("disabled configuration lost: %#v, %v", configured, err)
	}
	visible, err := service.UserMenus(ctx, 42)
	versions := configurationVersions(t, fixture.db)
	if err != nil || len(visible) != 0 || !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("equal hidden configuration visible=%#v versions=%v error=%v", visible, versions, err)
	}
	for _, ids := range [][]uint{{menu.ID, 0}, {menu.ID, 999}} {
		if err := service.AssignRoleMenus(ctx, 7, ids); err == nil {
			t.Fatalf("invalid raw IDs accepted: %v", ids)
		}
	}
	authorization.protectedRoles = map[uint]bool{7: true}
	if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID}); err == nil {
		t.Fatal("equal protected-role assignment accepted")
	}
	configuredAfter, err := service.RoleMenus(ctx, 7)
	versions = configurationVersions(t, fixture.db)
	if err != nil || !reflect.DeepEqual(configured, configuredAfter) || !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("rejected configuration changed menus=%#v versions=%v error=%v", configuredAfter, versions, err)
	}
	authorization.protectedRoles[7] = false
	if err := service.AssignRoleMenus(ctx, 7, nil); err != nil {
		t.Fatal(err)
	}
	configuredAfter, err = service.RoleMenus(ctx, 7)
	beforeVersions[42]++
	versions = configurationVersions(t, fixture.db)
	if err != nil || len(configuredAfter) != 0 || !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("real removal menus=%#v versions=%v error=%v", configuredAfter, versions, err)
	}
}

func TestEqualBindingStillRejectsUnlinkableAPIs(t *testing.T) {
	for _, invalid := range []string{"zero", "missing", "disabled", "public"} {
		t.Run(invalid, func(t *testing.T) {
			fixture := newConfigurationNavigationFixture(t)
			service, apis := fixture.service, fixture.apis
			ctx := context.Background()
			menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Actions", Type: 3})
			if err != nil {
				t.Fatal(err)
			}
			apis.records[9] = navigation.APIRecord{ID: 9, Method: "GET", Path: "/api/admin/actions", Status: 1, NeedAuth: 1}
			if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "actions.read"); err != nil {
				t.Fatal(err)
			}
			before, err := service.MenuTree(ctx)
			if err != nil {
				t.Fatal(err)
			}
			ids := []uint{9}
			api := apis.records[9]
			switch invalid {
			case "zero":
				ids = append(ids, 0)
			case "missing":
				delete(apis.records, 9)
			case "disabled":
				api.Status = 0
				apis.records[9] = api
			case "public":
				api.NeedAuth = 0
				apis.records[9] = api
			}
			beforeVersions := configurationVersions(t, fixture.db)
			if err := service.AssignAPIs(ctx, menu.ID, ids, "actions.read"); err == nil {
				t.Fatal("invalid equal binding accepted")
			}
			if invalid != "zero" {
				if _, err := service.UpdateMenu(ctx, menu.ID, navigation.UpdateInput{PermissionCode: stringPointer("actions.read")}); err == nil {
					t.Fatal("menu update bypassed binding prerequisites")
				}
			}
			after, err := service.MenuTree(ctx)
			versions := configurationVersions(t, fixture.db)
			if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(versions, beforeVersions) {
				t.Fatalf("rejected binding changed menu=%#v versions=%v error=%v", after, versions, err)
			}
		})
	}
}

func TestAPISetChangeWithEqualCodesStillReplacesConfiguration(t *testing.T) {
	fixture := newConfigurationNavigationFixture(t)
	service, authorization, apis := fixture.service, fixture.authorization, fixture.apis
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Actions", PermissionCode: "actions.read", Type: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{9, 10} {
		apis.records[id] = navigation.APIRecord{ID: id, Method: "GET", Path: "/api/admin/actions/" + strconv.Itoa(int(id)), PermissionCode: "actions.read", Status: 1, NeedAuth: 1}
	}
	authorization.roles[7] = true
	authorization.roleUsers[7] = []uint{42}
	if err := service.AssignRoleMenus(ctx, 7, []uint{menu.ID}); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignAPIs(ctx, menu.ID, []uint{9}, "actions.read"); err != nil {
		t.Fatal(err)
	}
	beforeVersions := configurationVersions(t, fixture.db)
	if err := service.AssignAPIs(ctx, menu.ID, []uint{10}, "actions.read"); err != nil {
		t.Fatal(err)
	}
	bound, err := service.MenuAPIs(ctx, menu.ID)
	beforeVersions[42]++
	versions := configurationVersions(t, fixture.db)
	if err != nil || len(bound) != 1 || bound[0].ID != 10 || bound[0].PermissionCode != "actions.read" || !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("equal-code target replacement APIs=%#v versions=%v error=%v", bound, versions, err)
	}
}

// The first transaction is deliberately rolled back; between attempts a
// controlled SQL writer supplies its path, so only the final attempt may escape.
type configurationRetryRunner struct {
	base    navigation.TransactionRunner
	between func()
}

func (runner configurationRetryRunner) Run(ctx context.Context, operation func(context.Context) error) error {
	retry := errors.New("retry fixture")
	err := runner.base.Run(ctx, func(transactionContext context.Context) error {
		if err := operation(transactionContext); err != nil {
			return err
		}
		return retry
	})
	if !errors.Is(err, retry) {
		return err
	}
	runner.between()
	return runner.base.Run(ctx, operation)
}
func (runner configurationRetryRunner) AfterCommit(ctx context.Context, callback func()) {
	runner.base.AfterCommit(ctx, callback)
}

func TestMenuSyncResetsCreatedCountWhenRetryBecomesAllSkip(t *testing.T) {
	fixture := newConfigurationNavigationFixture(t)
	db := fixture.db
	ctx := context.Background()
	repository := navigation.NewGORMRepository(db)
	fixture.authorization.allUsers = []uint{42}
	runner := configurationRetryRunner{
		base: platformdatabase.NewTransactionRunner(db),
		between: func() {
			menu := navigation.Menu{Name: "Concurrent menu", Path: stringPointer("/retry"), Type: 1, Status: 1}
			if err := repository.CreateMenu(ctx, &menu); err != nil {
				t.Fatal(err)
			}
		},
	}
	capability := configurationAuthorization{Authorization: fixture.authorization, versions: fixture.versions}
	service := navigation.NewService(repository, runner, capability, fixture.apis)
	beforeVersions := configurationVersions(t, db)
	created, err := service.SyncMenus(ctx, []navigation.SyncItem{{Name: "Attempt menu", Path: "/retry"}})
	versions := configurationVersions(t, db)
	if err != nil || created != 0 || !reflect.DeepEqual(versions, beforeVersions) {
		t.Fatalf("successful all-skip retry created=%d error=%v versions=%v", created, err, versions)
	}
	menus, err := repository.ListMenus(ctx, false)
	if err != nil || len(menus) != 1 || menus[0].Name != "Concurrent menu" {
		t.Fatalf("retry storage facts=%#v error=%v", menus, err)
	}
}
