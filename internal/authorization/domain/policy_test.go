package domain_test

import (
	"testing"

	"admin/internal/authorization/domain"
)

func TestDataScopeValuesPreserveAllEmptyAndSelfSemantics(t *testing.T) {
	all, err := domain.ParseDataScope("")
	if err != nil || all != domain.DataScopeAll {
		t.Fatalf("empty scope = %q, %v; want all", all, err)
	}
	for _, value := range []domain.DataScope{
		domain.DataScopeAll,
		domain.DataScopeSelf,
		domain.DataScopeOrg,
		domain.DataScopeOrgAndChildren,
		domain.DataScopeCustom,
	} {
		parsed, err := domain.ParseDataScope(string(value))
		if err != nil || parsed != value {
			t.Fatalf("ParseDataScope(%q) = %q, %v", value, parsed, err)
		}
	}
	if _, err := domain.ParseDataScope("unknown"); err == nil {
		t.Fatal("invalid data scope accepted")
	}

	self := domain.UserScopeForSelf(42)
	if self.All || len(self.UserIDs) != 1 || self.UserIDs[0] != 42 {
		t.Fatalf("self user scope = %#v", self)
	}
	orgSelf := domain.OrganizationScopeForSelf()
	if orgSelf.All || len(orgSelf.OrganizationIDs) != 0 {
		t.Fatalf("self organization scope = %#v", orgSelf)
	}
	allUsers := domain.AllUsersScope()
	if !allUsers.All || len(allUsers.UserIDs) != 0 {
		t.Fatalf("all user scope = %#v", allUsers)
	}
}

func TestPermissionCodeNormalizesAndRejectsInvalidValues(t *testing.T) {
	code, err := domain.NewPermissionCode("  admin.users.get ")
	if err != nil || code.String() != "admin.users.get" {
		t.Fatalf("normalized code = %q, %v", code.String(), err)
	}
	if _, err := domain.NewPermissionCode(""); err == nil {
		t.Fatal("empty permission code accepted")
	}
	if _, err := domain.NewPermissionCode("\n"); err == nil {
		t.Fatal("control-only permission code accepted")
	}
}

func TestAdminRoleIsProtected(t *testing.T) {
	if !domain.IsProtectedRole("admin") {
		t.Fatal("admin role is not protected")
	}
	if domain.IsProtectedRole("administrator") {
		t.Fatal("non-admin role was protected")
	}
}
