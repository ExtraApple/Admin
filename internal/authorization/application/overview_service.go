package application

import (
	"context"
	"sort"
	"strings"

	"admin/internal/authorization/domain"
)

type authorizationOverviewSnapshot struct {
	scope AuthorizationOverviewScope
	facts AuthorizationOverviewFacts
}

func (service *Service) GetAuthorizationOverview(ctx context.Context, operatorID uint) (AuthorizationOverview, error) {
	snapshot, err := service.readAuthorizationOverviewSnapshot(ctx, operatorID)
	if err != nil {
		return AuthorizationOverview{}, err
	}
	risks := buildAuthorizationRisks(snapshot.facts, snapshot.scope)
	return AuthorizationOverview{
		Scope:   authorizationOverviewScopeResult(snapshot),
		Summary: buildAuthorizationSummary(snapshot.facts, snapshot.scope),
		Risks: AuthorizationRiskSummary{
			Items:   cloneAuthorizationRisks(limitAuthorizationRisks(risks, authorizationOverviewRiskLimit)),
			Total:   int64(len(risks)),
			Limit:   authorizationOverviewRiskLimit,
			HasMore: len(risks) > authorizationOverviewRiskLimit,
		},
	}, nil
}

func authorizationOverviewScopeResult(snapshot authorizationOverviewSnapshot) AuthorizationOverviewScope {
	return AuthorizationOverviewScope{DataScope: snapshot.scope.DataScope, OrganizationCount: len(snapshot.facts.Organizations)}
}

func (service *Service) ListAuthorizationRisks(ctx context.Context, operatorID uint, request AuthorizationRiskListRequest) (AuthorizationRiskPage, error) {
	if err := validateAuthorizationRiskListRequest(request); err != nil {
		return AuthorizationRiskPage{}, err
	}
	request = normalizeAuthorizationRiskListRequest(request)
	snapshot, err := service.readAuthorizationOverviewSnapshot(ctx, operatorID)
	if err != nil {
		return AuthorizationRiskPage{}, err
	}
	risks := filterAuthorizationRisks(buildAuthorizationRisks(snapshot.facts, snapshot.scope), request)
	start := (request.Page - 1) * request.Size
	if start >= len(risks) {
		return AuthorizationRiskPage{Scope: authorizationOverviewScopeResult(snapshot), List: []AuthorizationRisk{}, Total: int64(len(risks)), Page: request.Page, Size: request.Size}, nil
	}
	end := start + request.Size
	if end > len(risks) {
		end = len(risks)
	}
	return AuthorizationRiskPage{Scope: authorizationOverviewScopeResult(snapshot), List: cloneAuthorizationRisks(risks[start:end]), Total: int64(len(risks)), Page: request.Page, Size: request.Size}, nil
}

func (service *Service) readAuthorizationOverviewSnapshot(ctx context.Context, operatorID uint) (authorizationOverviewSnapshot, error) {
	if service.overview == nil {
		return authorizationOverviewSnapshot{}, NewError(CodeInternalError, nil)
	}
	if operatorID == 0 {
		return authorizationOverviewSnapshot{}, ErrInvalidUser
	}
	var snapshot authorizationOverviewSnapshot
	err := service.transactions.Run(ctx, func(tx context.Context) error {
		userScope, err := service.ResolveUserScope(tx, domain.Principal{UserID: operatorID})
		if err != nil {
			return err
		}
		organizationScope, err := service.ResolveOrganizationScope(tx, domain.Principal{UserID: operatorID})
		if err != nil {
			return err
		}
		dataScope, err := service.effectiveDataScope(tx, operatorID)
		if err != nil {
			return err
		}
		overviewScope := AuthorizationOverviewScope{
			DataScope:    string(dataScope),
			User:         userScope,
			Organization: organizationScope,
		}
		facts, err := service.overview.ReadAuthorizationFacts(tx, overviewScope)
		if err != nil {
			return err
		}
		facts = restrictAuthorizationOverviewFacts(facts.ensureArrays(), overviewScope)
		snapshot = authorizationOverviewSnapshot{scope: overviewScope, facts: facts}
		return nil
	})
	if err == nil {
		return snapshot, nil
	}
	if _, classified := CodeOf(err); classified {
		return authorizationOverviewSnapshot{}, err
	}
	return authorizationOverviewSnapshot{}, NewError(CodeInternalError, err)
}

func (service *Service) effectiveDataScope(ctx context.Context, operatorID uint) (domain.DataScope, error) {
	roles, err := service.repository.RolesForUser(ctx, operatorID)
	if err != nil {
		return "", wrapError(err)
	}
	result := domain.DataScopeSelf
	for _, role := range roles {
		if domain.IsProtectedRole(role.Code) || role.DataScope == domain.DataScopeAll {
			return domain.DataScopeAll, nil
		}
		if dataScopeRank(role.DataScope) > dataScopeRank(result) {
			result = role.DataScope
		}
	}
	return result, nil
}

func dataScopeRank(scope domain.DataScope) int {
	switch scope {
	case domain.DataScopeSelf:
		return 1
	case domain.DataScopeCustom:
		return 2
	case domain.DataScopeOrg:
		return 3
	case domain.DataScopeOrgAndChildren:
		return 4
	case domain.DataScopeAll:
		return 5
	default:
		return 0
	}
}
func restrictAuthorizationOverviewFacts(facts AuthorizationOverviewFacts, scope AuthorizationOverviewScope) AuthorizationOverviewFacts {
	visibleUsers := make(map[uint]struct{}, len(facts.Users))
	for _, user := range facts.Users {
		if scope.User.All || containsOverviewID(scope.User.UserIDs, user.User.ID) {
			visibleUsers[user.User.ID] = struct{}{}
		}
	}
	visibleOrganizations := make(map[uint]struct{}, len(facts.Organizations))
	organizations := make([]AuthorizationOverviewOrganizationFact, 0, len(facts.Organizations))
	for _, organization := range facts.Organizations {
		if scope.Organization.All || containsOverviewID(scope.Organization.OrganizationIDs, organization.ID) {
			visibleOrganizations[organization.ID] = struct{}{}
			organizations = append(organizations, organization)
		}
	}
	users := make([]AuthorizationOverviewUserFact, 0, len(visibleUsers))
	for _, user := range facts.Users {
		if _, visible := visibleUsers[user.User.ID]; !visible {
			continue
		}
		organizationIDs := make([]uint, 0, len(user.OrganizationIDs))
		for _, organizationID := range user.OrganizationIDs {
			if _, visible := visibleOrganizations[organizationID]; visible {
				organizationIDs = append(organizationIDs, organizationID)
			}
		}
		user.OrganizationIDs = organizationIDs
		if user.RoleIDs == nil {
			user.RoleIDs = []uint{}
		}
		if user.OrganizationIDs == nil {
			user.OrganizationIDs = []uint{}
		}
		users = append(users, user)
	}
	roles := make([]AuthorizationOverviewRoleFact, 0, len(facts.Roles))
	for _, role := range facts.Roles {
		if !scope.User.All {
			role.UserIDs = filterOverviewIDsBySet(role.UserIDs, visibleUsers)
			if len(role.UserIDs) == 0 {
				continue
			}
		}
		if role.UserIDs == nil {
			role.UserIDs = []uint{}
		}
		if role.MenuIDs == nil {
			role.MenuIDs = []uint{}
		}
		if role.PermissionCodes == nil {
			role.PermissionCodes = []string{}
		}
		roles = append(roles, role)
	}
	facts.Roles = roles
	facts.Users = users
	facts.Organizations = organizations
	return facts
}

func buildAuthorizationSummary(facts AuthorizationOverviewFacts, scope AuthorizationOverviewScope) AuthorizationOverviewSummary {
	facts = facts.ensureArrays()
	roles := AuthorizationOverviewCount{Total: len(facts.Roles)}
	for _, role := range facts.Roles {
		if role.Status == 1 {
			roles.Enabled++
		}
		if len(role.UserIDs) > 0 {
			roles.Used++
		}
	}
	users := AuthorizationOverviewCount{Total: len(facts.Users)}
	for _, user := range facts.Users {
		if user.User.Status == 1 {
			users.Enabled++
		}
		if len(user.RoleIDs) == 0 {
			users.WithoutRole++
		}
	}
	organizations := AuthorizationOverviewCount{Total: len(facts.Organizations)}
	for _, organization := range facts.Organizations {
		if organization.Manageable {
			organizations.Manageable++
		}
	}
	_ = scope
	return AuthorizationOverviewSummary{Roles: roles, Users: users, Organizations: organizations}
}

func buildAuthorizationRisks(facts AuthorizationOverviewFacts, scope AuthorizationOverviewScope) []AuthorizationRisk {
	facts = facts.ensureArrays()
	visibleUsers := make(map[uint]AuthorizationOverviewUserFact, len(facts.Users))
	for _, user := range facts.Users {
		if scope.User.All || containsOverviewID(scope.User.UserIDs, user.User.ID) {
			user.OrganizationIDs = visibleOrganizationIDs(user.OrganizationIDs, scope.Organization)
			visibleUsers[user.User.ID] = user
		}
	}
	visibleOrganizations := make(map[uint]struct{}, len(facts.Organizations))
	for _, organization := range facts.Organizations {
		if scope.Organization.All || containsOverviewID(scope.Organization.OrganizationIDs, organization.ID) {
			visibleOrganizations[organization.ID] = struct{}{}
		}
	}
	menus := make(map[uint]AuthorizationOverviewMenuFact, len(facts.Menus))
	for _, menu := range facts.Menus {
		menus[menu.ID] = menu
	}
	risks := make([]AuthorizationRisk, 0)
	for _, role := range facts.Roles {
		userIDs := filterOverviewIDs(role.UserIDs, visibleUsers)
		if !scope.User.All && len(userIDs) == 0 {
			continue
		}
		permissionSet := make(map[string]struct{}, len(role.PermissionCodes))
		for _, code := range role.PermissionCodes {
			permissionSet[code] = struct{}{}
		}
		disabledMenus := 0
		missingPermissions := 0
		for _, menuID := range role.MenuIDs {
			menu, exists := menus[menuID]
			if !exists {
				continue
			}
			if menu.Status != 1 {
				disabledMenus++
			}
			if menu.PermissionCode != "" {
				if _, exists := permissionSet[menu.PermissionCode]; !exists {
					missingPermissions++
				}
			}
		}
		kinds := make([]RiskKind, 0, 3)
		issueCount := 0
		if disabledMenus > 0 {
			kinds = append(kinds, RiskKindDisabledAssignedMenu)
			issueCount += disabledMenus
		}
		if missingPermissions > 0 {
			kinds = append(kinds, RiskKindMissingMenuPermission)
			issueCount += missingPermissions
		}
		if role.Status == 1 && len(userIDs) > 0 && len(role.PermissionCodes) == 0 {
			kinds = append(kinds, RiskKindUsedRoleWithoutPermissions)
			issueCount++
		}
		if issueCount == 0 {
			continue
		}
		risks = append(risks, AuthorizationRisk{
			Resource: RiskResourceRole, ResourceID: role.ID, ResourceName: role.Name,
			IssueCount: issueCount, IssueKinds: kinds,
			PotentiallyAffectedUsers:         len(userIDs),
			PotentiallyAffectedOrganizations: affectedOrganizationsForRole(role, userIDs, visibleUsers, visibleOrganizations),
			SessionPolicy:                    SessionPolicy{RevalidateOnNextRequest: true},
		})
	}
	for _, user := range visibleUsers {
		if len(user.RoleIDs) != 0 {
			continue
		}
		name := user.User.Nickname
		if strings.TrimSpace(name) == "" {
			name = user.User.Username
		}
		risks = append(risks, AuthorizationRisk{
			Resource: RiskResourceUser, ResourceID: user.User.ID, ResourceName: name,
			IssueCount: 1, IssueKinds: []RiskKind{RiskKindManagedUserWithoutRole},
			PotentiallyAffectedUsers: 1, PotentiallyAffectedOrganizations: len(user.OrganizationIDs),
			SessionPolicy: SessionPolicy{RevalidateOnNextRequest: true},
		})
	}
	sort.SliceStable(risks, func(left, right int) bool {
		if risks[left].PotentiallyAffectedUsers != risks[right].PotentiallyAffectedUsers {
			return risks[left].PotentiallyAffectedUsers > risks[right].PotentiallyAffectedUsers
		}
		if risks[left].PotentiallyAffectedOrganizations != risks[right].PotentiallyAffectedOrganizations {
			return risks[left].PotentiallyAffectedOrganizations > risks[right].PotentiallyAffectedOrganizations
		}
		if risks[left].Resource != risks[right].Resource {
			return risks[left].Resource < risks[right].Resource
		}
		return risks[left].ResourceID < risks[right].ResourceID
	})
	return risks
}

func affectedOrganizationsForRole(role AuthorizationOverviewRoleFact, userIDs []uint, users map[uint]AuthorizationOverviewUserFact, visibleOrganizations map[uint]struct{}) int {
	if role.DataScope == string(domain.DataScopeAll) {
		return len(visibleOrganizations)
	}
	organizations := make(map[uint]struct{})
	for _, userID := range userIDs {
		for _, organizationID := range users[userID].OrganizationIDs {
			organizations[organizationID] = struct{}{}
		}
	}
	return len(organizations)
}

func filterAuthorizationRisks(risks []AuthorizationRisk, request AuthorizationRiskListRequest) []AuthorizationRisk {
	if request.Kind == "" && request.Resource == "" && request.Keyword == "" {
		return risks
	}
	keyword := strings.ToLower(request.Keyword)
	filtered := make([]AuthorizationRisk, 0, len(risks))
	for _, risk := range risks {
		if request.Resource != "" && risk.Resource != request.Resource {
			continue
		}
		if request.Kind != "" && !containsRiskKind(risk.IssueKinds, request.Kind) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(risk.ResourceName), keyword) {
			continue
		}
		filtered = append(filtered, risk)
	}
	return filtered
}

func limitAuthorizationRisks(risks []AuthorizationRisk, limit int) []AuthorizationRisk {
	if len(risks) > limit {
		return risks[:limit]
	}
	return risks
}

func cloneAuthorizationRisks(risks []AuthorizationRisk) []AuthorizationRisk {
	result := make([]AuthorizationRisk, len(risks))
	for index, risk := range risks {
		result[index] = risk
		result[index].IssueKinds = append([]RiskKind(nil), risk.IssueKinds...)
	}
	if result == nil {
		return []AuthorizationRisk{}
	}
	return result
}

func containsRiskKind(kinds []RiskKind, target RiskKind) bool {
	for _, kind := range kinds {
		if kind == target {
			return true
		}
	}
	return false
}

func containsOverviewID(ids []uint, target uint) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func filterOverviewIDs(ids []uint, users map[uint]AuthorizationOverviewUserFact) []uint {
	result := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := users[id]; !exists {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
func filterOverviewIDsBySet(ids []uint, visible map[uint]struct{}) []uint {
	result := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := visible[id]; !exists {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func visibleOrganizationIDs(ids []uint, scope domain.OrganizationScope) []uint {
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if scope.All || containsOverviewID(scope.OrganizationIDs, id) {
			result = append(result, id)
		}
	}
	return result
}
