package application_test

import (
	"context"
	"testing"

	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/organization"
)

func TestAuthorizationOverviewAppliesOperatorScopeBeforeSummaryAndRisks(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	unit := organization.Unit{Name: "Visible", Code: "visible", Status: 1}
	outside := organization.Unit{Name: "Outside", Code: "outside", Status: 1}
	if err := fixture.organizations.CreateUnit(ctx, &unit); err != nil {
		t.Fatalf("create visible organization: %v", err)
	}
	if err := fixture.organizations.CreateUnit(ctx, &outside); err != nil {
		t.Fatalf("create outside organization: %v", err)
	}
	if err := fixture.organizations.CreateMemberships(ctx, []organization.Membership{
		{UserID: 1, OrganizationID: unit.ID}, {UserID: 10, OrganizationID: unit.ID}, {UserID: 20, OrganizationID: outside.ID},
	}); err != nil {
		t.Fatalf("create organization memberships: %v", err)
	}
	manager := domain.Role{Name: "Manager", Code: "manager", Status: 1, DataScope: domain.DataScopeOrg}
	if err := fixture.repository.CreateRole(ctx, &manager); err != nil {
		t.Fatalf("create manager role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, manager.ID, []uint{1}); err != nil {
		t.Fatalf("assign manager role: %v", err)
	}
	reader := &overviewReaderFake{facts: application.AuthorizationOverviewFacts{
		Roles: []application.AuthorizationOverviewRoleFact{
			{ID: manager.ID, Name: manager.Name, Code: manager.Code, Status: 1, DataScope: string(domain.DataScopeOrg), UserIDs: []uint{1}, PermissionCodes: []string{"users.read"}},
			{ID: 99, Name: "Outside role", Code: "outside", Status: 1, DataScope: string(domain.DataScopeSelf), UserIDs: []uint{20}, PermissionCodes: []string{}},
		},
		Users: []application.AuthorizationOverviewUserFact{
			{User: identitydomain.DirectoryUser{ID: 1, Username: "operator", Status: 1}, RoleIDs: []uint{manager.ID}, OrganizationIDs: []uint{unit.ID}},
			{User: identitydomain.DirectoryUser{ID: 10, Username: "visible", Status: 1}, RoleIDs: []uint{}, OrganizationIDs: []uint{unit.ID}},
			{User: identitydomain.DirectoryUser{ID: 20, Username: "outside", Status: 1}, RoleIDs: []uint{}, OrganizationIDs: []uint{outside.ID}},
		},
		Organizations: []application.AuthorizationOverviewOrganizationFact{
			{ID: unit.ID, Name: unit.Name, Manageable: true}, {ID: outside.ID, Name: outside.Name, Manageable: true},
		},
	}}
	service := newOverviewService(fixture, reader)

	overview, err := service.GetAuthorizationOverview(ctx, 1)
	if err != nil {
		t.Fatalf("get scoped overview: %v", err)
	}
	if overview.Scope.DataScope != string(domain.DataScopeOrg) || overview.Scope.OrganizationCount != 1 {
		t.Fatalf("scoped overview scope = %#v", overview.Scope)
	}
	if overview.Summary.Users.Total != 2 || overview.Summary.Users.WithoutRole != 1 || overview.Summary.Organizations.Total != 1 {
		t.Fatalf("scoped overview summary = %#v", overview.Summary)
	}
	if overview.Risks.Total != 1 || len(overview.Risks.Items) != 1 || overview.Risks.Items[0].ResourceID != 10 {
		t.Fatalf("scoped overview risks = %#v", overview.Risks)
	}
}
