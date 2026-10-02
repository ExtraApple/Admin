package application_test

import (
	"context"
	"testing"

	"admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/organization"
)

type overviewReaderFake struct {
	facts  application.AuthorizationOverviewFacts
	scopes []application.AuthorizationOverviewScope
}

func (reader *overviewReaderFake) ReadAuthorizationFacts(_ context.Context, scope application.AuthorizationOverviewScope) (application.AuthorizationOverviewFacts, error) {
	reader.scopes = append(reader.scopes, scope)
	return reader.facts, nil
}

func newOverviewService(fixture authorizationFixture, reader application.AuthorizationOverviewReader) *application.Service {
	return application.NewService(
		fixture.repository,
		fixture.transactions,
		organizationScopeReaderForTest(fixture),
		fixture.users,
		fixture.versions,
		reader,
	)
}

func organizationScopeReaderForTest(fixture authorizationFixture) application.OrganizationScopeReader {
	return organizationScopeAdapter{fixture: fixture}
}

type organizationScopeAdapter struct{ fixture authorizationFixture }

func (adapter organizationScopeAdapter) MemberOrganizationIDs(ctx context.Context, userID uint) ([]uint, error) {
	return adapter.fixture.organizations.MemberOrganizationIDs(ctx, userID)
}
func (adapter organizationScopeAdapter) DescendantOrganizationIDs(ctx context.Context, ids []uint) ([]uint, error) {
	return organization.NewHierarchy(adapter.fixture.organizations).DescendantOrganizationIDs(ctx, ids)
}
func (adapter organizationScopeAdapter) ExistingOrganizationIDs(ctx context.Context, ids []uint) ([]uint, error) {
	return organization.NewScopeReader(adapter.fixture.organizations, organization.NewHierarchy(adapter.fixture.organizations)).ExistingOrganizationIDs(ctx, ids)
}
func (adapter organizationScopeAdapter) MemberUserIDs(ctx context.Context, organizationID uint) ([]uint, error) {
	return adapter.fixture.organizations.MemberUserIDs(ctx, organizationID)
}

func TestListAuthorizationRisksAggregatesRulesAndUsesStableOrdering(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	admin := domain.Role{Name: "Admin", Code: "admin", Status: 1, DataScope: domain.DataScopeAll}
	if err := fixture.repository.CreateRole(ctx, &admin); err != nil {
		t.Fatalf("create admin role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, admin.ID, []uint{1}); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}

	reader := &overviewReaderFake{facts: application.AuthorizationOverviewFacts{
		Roles: []application.AuthorizationOverviewRoleFact{
			{ID: admin.ID, Name: admin.Name, Code: admin.Code, Status: admin.Status, DataScope: string(admin.DataScope), UserIDs: []uint{1}, PermissionCodes: []string{"admin.read"}},
			{ID: 2, Name: "Disabled menu", Code: "disabled-menu", Status: 1, DataScope: string(domain.DataScopeSelf), UserIDs: []uint{10}, MenuIDs: []uint{20}, PermissionCodes: []string{"menu.read"}},
			{ID: 3, Name: "Missing permission", Code: "missing-permission", Status: 1, DataScope: string(domain.DataScopeSelf), UserIDs: []uint{10}, MenuIDs: []uint{21}, PermissionCodes: []string{}},
			{ID: 4, Name: "Used without permission", Code: "used-without-permission", Status: 1, DataScope: string(domain.DataScopeSelf), UserIDs: []uint{11}, PermissionCodes: []string{}},
		},
		Menus: []application.AuthorizationOverviewMenuFact{
			{ID: 20, Name: "Disabled", Status: 0, PermissionCode: ""},
			{ID: 21, Name: "Requires permission", Status: 1, PermissionCode: "missing.read"},
		},
		Users: []application.AuthorizationOverviewUserFact{
			{User: identitydomain.DirectoryUser{ID: 1, Username: "admin", Status: 1}, RoleIDs: []uint{admin.ID}, OrganizationIDs: []uint{100}},
			{User: identitydomain.DirectoryUser{ID: 10, Username: "operator", Status: 1}, RoleIDs: []uint{2, 3}, OrganizationIDs: []uint{100}},
			{User: identitydomain.DirectoryUser{ID: 11, Username: "unassigned", Status: 1}, RoleIDs: []uint{}, OrganizationIDs: []uint{101}},
		},
		Organizations: []application.AuthorizationOverviewOrganizationFact{
			{ID: 100, Name: "North", Manageable: true},
			{ID: 101, Name: "South", Manageable: true},
		},
	}}
	service := newOverviewService(fixture, reader)

	page, err := service.ListAuthorizationRisks(ctx, 1, application.AuthorizationRiskListRequest{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("list authorization risks: %v", err)
	}
	if page.Total != 4 || len(page.List) != 4 || page.Page != 1 || page.Size != 20 {
		t.Fatalf("risk page = %#v", page)
	}
	if got := page.List[0]; got.Resource != "role" || got.ResourceID != 2 || got.IssueCount != 1 || len(got.IssueKinds) != 1 || got.IssueKinds[0] != application.RiskKindDisabledAssignedMenu {
		t.Fatalf("first risk = %#v", got)
	}
	if got := page.List[1]; got.Resource != "role" || got.ResourceID != 3 || got.IssueKinds[0] != application.RiskKindMissingMenuPermission {
		t.Fatalf("second risk = %#v", got)
	}
	if got := page.List[2]; got.Resource != "role" || got.ResourceID != 4 || got.IssueKinds[0] != application.RiskKindUsedRoleWithoutPermissions {
		t.Fatalf("third risk = %#v", got)
	}
	if got := page.List[3]; got.Resource != "user" || got.ResourceID != 11 || got.IssueKinds[0] != application.RiskKindManagedUserWithoutRole {
		t.Fatalf("fourth risk = %#v", got)
	}
	if len(reader.scopes) != 1 || !reader.scopes[0].User.All || !reader.scopes[0].Organization.All || reader.scopes[0].DataScope != string(domain.DataScopeAll) {
		t.Fatalf("overview scope = %#v", reader.scopes)
	}
}

func TestGetAuthorizationOverviewReturnsSummaryTopTenAndEmptySafeArrays(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	admin := domain.Role{Name: "Admin", Code: "admin", Status: 1, DataScope: domain.DataScopeAll}
	if err := fixture.repository.CreateRole(ctx, &admin); err != nil {
		t.Fatalf("create admin role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, admin.ID, []uint{1}); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	users := make([]application.AuthorizationOverviewUserFact, 12)
	for index := range users {
		users[index] = application.AuthorizationOverviewUserFact{
			User: identitydomain.DirectoryUser{ID: uint(index + 10), Username: "user", Status: 1},
		}
	}
	reader := &overviewReaderFake{facts: application.AuthorizationOverviewFacts{
		Roles:         []application.AuthorizationOverviewRoleFact{{ID: admin.ID, Name: admin.Name, Code: admin.Code, Status: 1, DataScope: string(domain.DataScopeAll), UserIDs: []uint{1}, PermissionCodes: []string{"admin.read"}}},
		Users:         users,
		Organizations: []application.AuthorizationOverviewOrganizationFact{{ID: 1, Name: "HQ", Manageable: true}},
	}}
	service := newOverviewService(fixture, reader)

	overview, err := service.GetAuthorizationOverview(ctx, 1)
	if err != nil {
		t.Fatalf("get authorization overview: %v", err)
	}
	if overview.Summary.Roles.Total != 1 || overview.Summary.Roles.Enabled != 1 || overview.Summary.Roles.Used != 1 {
		t.Fatalf("role summary = %#v", overview.Summary.Roles)
	}
	if overview.Summary.Users.Total != 12 || overview.Summary.Users.Enabled != 12 || overview.Summary.Users.WithoutRole != 12 {
		t.Fatalf("user summary = %#v", overview.Summary.Users)
	}
	if overview.Risks.Total != 12 || len(overview.Risks.Items) != 10 || overview.Risks.Limit != 10 || !overview.Risks.HasMore {
		t.Fatalf("overview risks = %#v", overview.Risks)
	}
	if overview.Risks.Items[0].IssueKinds == nil || overview.Risks.Items[0].SessionPolicy.RevalidateOnNextRequest != true {
		t.Fatalf("overview risk policy = %#v", overview.Risks.Items[0])
	}
	if overview.Scope.DataScope != string(domain.DataScopeAll) || overview.Scope.OrganizationCount != 1 {
		t.Fatalf("overview scope = %#v", overview.Scope)
	}

	page, err := service.ListAuthorizationRisks(ctx, 1, application.AuthorizationRiskListRequest{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("list authorization risks: %v", err)
	}
	if page.Scope.DataScope != overview.Scope.DataScope || page.Scope.OrganizationCount != overview.Scope.OrganizationCount {
		t.Fatalf("risk page scope = %#v, overview scope = %#v", page.Scope, overview.Scope)
	}

	emptyReader := &overviewReaderFake{facts: application.AuthorizationOverviewFacts{}}
	emptyService := newOverviewService(fixture, emptyReader)
	empty, err := emptyService.ListAuthorizationRisks(ctx, 1, application.AuthorizationRiskListRequest{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("list empty authorization risks: %v", err)
	}
	if empty.List == nil || len(empty.List) != 0 || empty.Total != 0 {
		t.Fatalf("empty risk page = %#v", empty)
	}
}

func TestListAuthorizationRisksValidatesAndFilters(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	admin := domain.Role{Name: "Admin", Code: "admin", Status: 1, DataScope: domain.DataScopeAll}
	if err := fixture.repository.CreateRole(ctx, &admin); err != nil {
		t.Fatalf("create admin role: %v", err)
	}
	if err := fixture.repository.ReplaceRoleUsers(ctx, admin.ID, []uint{1}); err != nil {
		t.Fatalf("assign admin role: %v", err)
	}
	reader := &overviewReaderFake{facts: application.AuthorizationOverviewFacts{
		Roles: []application.AuthorizationOverviewRoleFact{
			{ID: admin.ID, Name: admin.Name, Code: admin.Code, Status: 1, DataScope: string(domain.DataScopeAll), UserIDs: []uint{1}, PermissionCodes: []string{"admin.read"}},
			{ID: 2, Name: "Missing permission", Code: "missing-permission", Status: 1, DataScope: string(domain.DataScopeSelf), UserIDs: []uint{10}, MenuIDs: []uint{20}},
		},
		Menus: []application.AuthorizationOverviewMenuFact{{ID: 20, Name: "Users", Status: 1, PermissionCode: "users.read"}},
		Users: []application.AuthorizationOverviewUserFact{{User: identitydomain.DirectoryUser{ID: 1, Username: "admin", Status: 1}, RoleIDs: []uint{admin.ID}}, {User: identitydomain.DirectoryUser{ID: 10, Username: "operator", Status: 1}, RoleIDs: []uint{2}}},
	}}
	service := newOverviewService(fixture, reader)

	filtered, err := service.ListAuthorizationRisks(ctx, 1, application.AuthorizationRiskListRequest{Page: 1, Size: 10, Kind: application.RiskKindMissingMenuPermission, Resource: application.RiskResourceRole, Keyword: "Missing"})
	if err != nil {
		t.Fatalf("filter authorization risks: %v", err)
	}
	if filtered.Total != 1 || len(filtered.List) != 1 || filtered.List[0].ResourceID != 2 {
		t.Fatalf("filtered risks = %#v", filtered)
	}
	for _, request := range []application.AuthorizationRiskListRequest{{Page: 0, Size: 10}, {Page: 1, Size: 0}, {Page: 1, Size: 101}, {Page: 1, Size: 10, Kind: "unknown"}, {Page: 1, Size: 10, Resource: "permission"}} {
		if _, err := service.ListAuthorizationRisks(ctx, 1, request); err == nil {
			t.Fatalf("invalid risk request accepted: %#v", request)
		}
	}
}
