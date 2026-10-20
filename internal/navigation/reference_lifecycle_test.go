package navigation_test

import (
	"context"
	"reflect"
	"testing"

	"admin/internal/navigation"
)

func TestMissingMenuLifecycleTargetsRetainNotFoundClassification(t *testing.T) {
	service, authorization := newNavigationServiceFixture(t)
	ctx := context.Background()
	parent, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Removed parent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteMenu(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	child, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Existing root"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.MenuTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []struct {
		name string
		run  func() error
	}{
		{"create under removed parent", func() error {
			_, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Rejected child", ParentID: parent.ID})
			return err
		}},
		{"move under removed parent", func() error {
			_, err := service.UpdateMenu(ctx, child.ID, navigation.UpdateInput{ParentID: &parent.ID})
			return err
		}},
		{"delete removed target", func() error { return service.DeleteMenu(ctx, parent.ID) }},
	} {
		t.Run(action.name, func(t *testing.T) {
			err := action.run()
			if code, ok := navigation.CodeOf(err); !ok || code != navigation.CodeNotFound {
				t.Fatalf("missing lifecycle target = %v", err)
			}
			after, err := service.MenuTree(ctx)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected operation changed menus: %#v, %v", after, err)
			}
		})
	}
	authorization.roles[7] = true
	if err := service.AssignRoleMenus(ctx, 7, []uint{parent.ID}); err == nil {
		t.Fatal("removed menu was bound")
	} else if code, _ := navigation.CodeOf(err); code != navigation.CodeValidationInvalid {
		t.Fatalf("invalid menu binding classification = %v", err)
	}
}
