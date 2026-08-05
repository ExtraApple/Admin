package navigation_test

import (
	"context"
	"testing"

	"admin/testsupport/testutil"
	"admin/internal/navigation"
	platformdatabase "admin/internal/platform/database"
)

func TestRepositoryOwnsMenuAndRelationshipPersistence(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(navigation.Models()...); err != nil {
		t.Fatalf("migrate navigation models: %v", err)
	}
	repository := navigation.NewGORMRepository(db)
	transactions := platformdatabase.NewTransactionRunner(db)
	ctx := context.Background()

	parentPath := "/settings"
	parent := navigation.Menu{Name: "Settings", Path: &parentPath, Sort: 2, Type: 1, Status: 1}
	childPath := "/settings/users"
	child := navigation.Menu{ParentID: 0, Name: "Users", Path: &childPath, Sort: 1, Type: 2, Status: 1}
	if err := transactions.Run(ctx, func(txCtx context.Context) error {
		if err := repository.CreateMenu(txCtx, &parent); err != nil {
			return err
		}
		child.ParentID = parent.ID
		return repository.CreateMenu(txCtx, &child)
	}); err != nil {
		t.Fatalf("create menu hierarchy: %v", err)
	}

	menus, err := repository.ListMenus(ctx, false)
	if err != nil || len(menus) != 2 || menus[0].ID != child.ID || menus[1].ID != parent.ID {
		t.Fatalf("ordered menus = %#v, %v", menus, err)
	}
	if exists, err := repository.PathExists(ctx, childPath, 0); err != nil || !exists {
		t.Fatalf("PathExists = %v, %v; want true", exists, err)
	}
	if _, err := repository.LockMenu(ctx, child.ID); err != nil {
		t.Fatalf("lock menu: %v", err)
	}

	if err := transactions.Run(ctx, func(txCtx context.Context) error {
		if err := repository.ReplaceRoleMenus(txCtx, 7, []uint{parent.ID, child.ID, child.ID}); err != nil {
			return err
		}
		return repository.ReplaceMenuAPIs(txCtx, child.ID, []uint{11, 12, 12})
	}); err != nil {
		t.Fatalf("replace navigation relationships: %v", err)
	}

	menuIDs, err := repository.MenuIDsByRoleIDs(ctx, []uint{7})
	if err != nil || len(menuIDs) != 2 || menuIDs[0] != parent.ID || menuIDs[1] != child.ID {
		t.Fatalf("role menu IDs = %#v, %v", menuIDs, err)
	}
	roleIDs, err := repository.RoleIDsByMenuIDs(ctx, []uint{child.ID})
	if err != nil || len(roleIDs) != 1 || roleIDs[0] != 7 {
		t.Fatalf("menu role IDs = %#v, %v", roleIDs, err)
	}
	apiIDs, err := repository.APIIDsByMenuIDs(ctx, []uint{child.ID}, false)
	if err != nil || len(apiIDs) != 2 || apiIDs[0] != 11 || apiIDs[1] != 12 {
		t.Fatalf("menu API IDs = %#v, %v", apiIDs, err)
	}
	menuIDs, err = repository.MenuIDsByAPIIDs(ctx, []uint{12}, false)
	if err != nil || len(menuIDs) != 1 || menuIDs[0] != child.ID {
		t.Fatalf("API menu IDs = %#v, %v", menuIDs, err)
	}

	if err := repository.UpdateMenusPermissionCode(ctx, []uint{child.ID}, "user.read"); err != nil {
		t.Fatalf("update permission code: %v", err)
	}
	if count, err := repository.CountPermissionCode(ctx, "user.read"); err != nil || count != 1 {
		t.Fatalf("permission code count = %d, %v; want 1", count, err)
	}
	if err := repository.DeleteMenu(ctx, child.ID); err != nil {
		t.Fatalf("delete menu: %v", err)
	}
	if apiIDs, err := repository.APIIDsByMenuIDs(ctx, []uint{child.ID}, false); err != nil || len(apiIDs) != 0 {
		t.Fatalf("deleted menu API IDs = %#v, %v; want empty", apiIDs, err)
	}
	if roleIDs, err := repository.RoleIDsByMenuIDs(ctx, []uint{child.ID}); err != nil || len(roleIDs) != 0 {
		t.Fatalf("deleted menu role IDs = %#v, %v; want empty", roleIDs, err)
	}
}

func TestModelsKeepExistingTableNames(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(navigation.Models()...); err != nil {
		t.Fatalf("migrate navigation models: %v", err)
	}
	for _, table := range []string{"menus", "role_menus", "menu_apis"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("missing Navigation-owned table %q", table)
		}
	}
}
