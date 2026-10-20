package navigation_test

import (
	"context"
	"reflect"
	"testing"

	"admin/internal/navigation"
)

func TestMenuApplicationRejectsExplicitEmptyNameBeforeClearing(t *testing.T) {
	service, _ := newNavigationServiceFixture(t)
	ctx := context.Background()
	menu, err := service.CreateMenu(ctx, navigation.CreateInput{Name: "Keep name", Icon: "keep-icon", Component: "KeepComponent", Type: 3})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.MenuTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	_, err = service.UpdateMenu(ctx, menu.ID, navigation.UpdateInput{Name: &empty, Icon: &empty})
	if code, found := navigation.CodeOf(err); !found || code != navigation.CodeValidationInvalid {
		t.Fatalf("direct explicit empty name error=%v", err)
	}
	after, err := service.MenuTree(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected direct update changed menu: before=%#v after=%#v error=%v", before, after, err)
	}
}
