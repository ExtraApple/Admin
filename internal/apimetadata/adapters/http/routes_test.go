package httpadapter_test

import (
	"testing"

	apihttp "admin/internal/apimetadata/adapters/http"
	"admin/internal/routecatalog"
)

func TestRoutesDescribeAPIMetadataHTTPSurface(t *testing.T) {
	descriptors := apihttp.Routes(nil)
	catalog, err := routecatalog.New(descriptors)
	if err != nil {
		t.Fatalf("build API Metadata Route Catalog: %v", err)
	}
	for _, path := range []string{
		"/api/admin/api-groups", "/api/admin/api-methods", "/api/admin/apis", "/api/admin/apis/:id", "/api/admin/apis/:id/menu-button",
		"/api/admin/apis/sync", "/api/admin/apis/sync-permissions",
	} {
		found := false
		for _, descriptor := range catalog.Snapshot() {
			if descriptor.Path == path {
				found = true
				if descriptor.Handler == nil || descriptor.Access != routecatalog.PermissionControlled {
					t.Fatalf("descriptor %s has invalid handler or access", path)
				}
			}
		}
		if !found {
			t.Fatalf("missing API Metadata route %s", path)
		}
	}
}
