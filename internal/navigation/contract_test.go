package navigation_test

import (
	"context"
	"testing"

	"admin/internal/navigation"
)

func TestMenuTypeContractMatrix(t *testing.T) {
	service, _ := newNavigationServiceFixture(t)
	ctx := context.Background()
	directory, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Directory", Type: 1})
	if err != nil {
		t.Fatalf("create directory: %v", err)
	}
	page, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Page", Path: "/contract-page", Type: 2, ParentID: directory.ID})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	if _, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Button", Type: 3, ParentID: page.ID}); err != nil {
		t.Fatalf("create button: %v", err)
	}
	for _, candidate := range []navigation.CreateInput{
		{Name: "Directory with path", Type: 1, Path: "/invalid-directory"},
		{Name: "Page without path", Type: 2},
		{Name: "Button with component", Type: 3, Component: "Invalid"},
	} {
		_, err := service.CreateMenu(ctx, candidate)
		code, ok := navigation.CodeOf(err)
		if !ok || code != navigation.CodeValidationInvalid {
			t.Fatalf("invalid menu candidate %#v returned %v", candidate, err)
		}
	}
}

func TestSyncMenusBuildsGraphBeforeWriting(t *testing.T) {
	service, _ := newNavigationServiceFixture(t)
	ctx := context.Background()
	created, err := service.SyncMenus(ctx, []navigation.SyncItem{
		{Name: "Child", Path: "/sync/child", ParentPath: "/sync", Type: 2},
		{Name: "Parent", Path: "/sync", Type: 2},
	})
	if err != nil || created != 2 {
		t.Fatalf("topological sync created=%d error=%v", created, err)
	}
	menus, err := service.MenuTree(ctx)
	if err != nil || len(menus) != 1 || len(menus[0].Children) != 1 {
		t.Fatalf("synced menu tree=%#v error=%v", menus, err)
	}

	before := len(menus)
	_, err = service.SyncMenus(ctx, []navigation.SyncItem{{Name: "Unknown", Path: "/sync/unknown", ParentPath: "/missing", Type: 2}})
	if code, ok := navigation.CodeOf(err); !ok || code != navigation.CodeConflict {
		t.Fatalf("unknown parent error=%v want %s", err, navigation.CodeConflict)
	}
	menus, err = service.MenuTree(ctx)
	if err != nil || len(menus) != before {
		t.Fatalf("unknown parent partially wrote tree=%#v error=%v", menus, err)
	}

	_, err = service.SyncMenus(ctx, []navigation.SyncItem{
		{Name: "Cycle A", Path: "/cycle/a", ParentPath: "/cycle/b", Type: 2},
		{Name: "Cycle B", Path: "/cycle/b", ParentPath: "/cycle/a", Type: 2},
	})
	if code, ok := navigation.CodeOf(err); !ok || code != navigation.CodeConflict {
		t.Fatalf("cycle error=%v want %s", err, navigation.CodeConflict)
	}
}
