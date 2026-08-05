package navigation_test

import (
	"testing"

	"admin/internal/navigation"
	"admin/internal/routecatalog"
)

func TestRoutesDescribeNavigationHTTPSurface(t *testing.T) {
	descriptors := navigation.Routes(nil)
	catalog, err := routecatalog.New(descriptors)
	if err != nil {
		t.Fatalf("build Navigation Route Catalog: %v", err)
	}
	for _, path := range []string{
		"/api/admin/menus", "/api/admin/menus/:id", "/api/admin/menus/:id/apis",
		"/api/admin/roles/:id/menus",
	} {
		found := false
		for _, descriptor := range catalog.Snapshot() {
			if descriptor.Path == path {
				found = true
				if descriptor.Access != routecatalog.PermissionControlled || descriptor.Handler == nil {
					t.Fatalf("descriptor %s has invalid access or handler", path)
				}
			}
		}
		if !found {
			t.Fatalf("missing Navigation route %s", path)
		}
	}
}
