package app

import (
	"context"

	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	"admin/internal/authorization/domain"
	"admin/internal/navigation"
	"admin/internal/organization"
)

type authorizationOverviewReader struct {
	authorization *authgorm.Repository
	navigation    *navigation.GORMRepository
	organizations organization.Repository
	users         authapplication.OverviewUserDirectory
}

func (reader authorizationOverviewReader) ReadAuthorizationFacts(ctx context.Context, scope authapplication.AuthorizationOverviewScope) (authapplication.AuthorizationOverviewFacts, error) {
	userIDs, err := reader.visibleUserIDs(ctx, scope.User)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	users, err := reader.users.ListUsersByIDs(ctx, userIDs)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	roleSummaries, err := reader.authorization.UserRoleSummaries(ctx, userIDs)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	roles, _, err := reader.authorization.ListRoles(ctx, 0, -1)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}

	userRoleIDs := make(map[uint][]uint, len(users))
	roleUserIDs := make(map[uint][]uint, len(roles))
	for _, summary := range roleSummaries {
		userRoleIDs[summary.UserID] = append(userRoleIDs[summary.UserID], summary.RoleID)
		roleUserIDs[summary.RoleID] = append(roleUserIDs[summary.RoleID], summary.UserID)
	}
	visibleRoleIDs := make([]uint, 0, len(roles))
	roleFacts := make([]authapplication.AuthorizationOverviewRoleFact, 0, len(roles))
	for _, role := range roles {
		assignedUsers := uniqueOverviewIDs(roleUserIDs[role.ID])
		if !scope.User.All && len(assignedUsers) == 0 {
			continue
		}
		visibleRoleIDs = append(visibleRoleIDs, role.ID)
		roleFacts = append(roleFacts, authapplication.AuthorizationOverviewRoleFact{
			ID: role.ID, Name: role.Name, Code: role.Code, Status: role.Status,
			DataScope: string(role.DataScope), UserIDs: assignedUsers,
		})
	}

	permissionFacts, err := reader.authorization.RolePermissionFacts(ctx, visibleRoleIDs)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	permissionsByRole := make(map[uint][]string, len(visibleRoleIDs))
	for _, fact := range permissionFacts {
		permissionsByRole[fact.RoleID] = append(permissionsByRole[fact.RoleID], fact.Code)
	}
	menuAssignments, err := reader.navigation.RoleMenuAssignments(ctx, visibleRoleIDs)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	menus, err := reader.navigation.ListMenus(ctx, false)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	menuFacts := make([]authapplication.AuthorizationOverviewMenuFact, 0, len(menus))
	menuIDs := make(map[uint]struct{}, len(menuAssignments))
	for _, assignment := range menuAssignments {
		menuIDs[assignment.MenuID] = struct{}{}
		for index := range roleFacts {
			if roleFacts[index].ID == assignment.RoleID {
				roleFacts[index].MenuIDs = append(roleFacts[index].MenuIDs, assignment.MenuID)
				break
			}
		}
	}
	for _, menu := range menus {
		if _, exists := menuIDs[menu.ID]; !exists {
			continue
		}
		permissionCode := menu.PermissionCode
		menuFacts = append(menuFacts, authapplication.AuthorizationOverviewMenuFact{ID: menu.ID, Name: menu.Name, Status: menu.Status, PermissionCode: permissionCode})
	}
	for index := range roleFacts {
		roleFacts[index].PermissionCodes = append([]string{}, permissionsByRole[roleFacts[index].ID]...)
		if roleFacts[index].UserIDs == nil {
			roleFacts[index].UserIDs = []uint{}
		}
		if roleFacts[index].MenuIDs == nil {
			roleFacts[index].MenuIDs = []uint{}
		}
		if roleFacts[index].PermissionCodes == nil {
			roleFacts[index].PermissionCodes = []string{}
		}
	}

	organizationFacts, visibleOrganizationIDs, err := reader.visibleOrganizations(ctx, scope.Organization)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	organizationFactsByUser, err := reader.organizations.UserOrganizationFacts(ctx, userIDs)
	if err != nil {
		return authapplication.AuthorizationOverviewFacts{}, err
	}
	allowedOrganizations := make(map[uint]struct{}, len(visibleOrganizationIDs))
	for _, organizationID := range visibleOrganizationIDs {
		allowedOrganizations[organizationID] = struct{}{}
	}
	userFacts := make([]authapplication.AuthorizationOverviewUserFact, 0, len(users))
	organizationsByUser := make(map[uint][]uint, len(users))
	for _, fact := range organizationFactsByUser {
		if _, allowed := allowedOrganizations[fact.OrganizationID]; !allowed {
			continue
		}
		organizationsByUser[fact.UserID] = append(organizationsByUser[fact.UserID], fact.OrganizationID)
	}
	for _, user := range users {
		userFacts = append(userFacts, authapplication.AuthorizationOverviewUserFact{
			User: user, RoleIDs: uniqueOverviewIDs(userRoleIDs[user.ID]), OrganizationIDs: uniqueOverviewIDs(organizationsByUser[user.ID]),
		})
	}
	return authapplication.AuthorizationOverviewFacts{
		Roles: roleFacts, Menus: menuFacts, Users: userFacts, Organizations: organizationFacts,
	}, nil
}

func (reader authorizationOverviewReader) visibleUserIDs(ctx context.Context, scope domain.UserScope) ([]uint, error) {
	if !scope.All {
		return append([]uint(nil), scope.UserIDs...), nil
	}
	ids, err := reader.users.ListUserIDs(ctx)
	if err != nil {
		return nil, err
	}
	return uniqueOverviewIDs(ids), nil
}

func (reader authorizationOverviewReader) visibleOrganizations(ctx context.Context, scope domain.OrganizationScope) ([]authapplication.AuthorizationOverviewOrganizationFact, []uint, error) {
	var units []organization.Unit
	var err error
	if scope.All {
		units, err = reader.organizations.ListAllUnits(ctx)
	} else {
		units, err = reader.organizations.ListUnitsByIDs(ctx, scope.OrganizationIDs)
	}
	if err != nil {
		return nil, nil, err
	}
	facts := make([]authapplication.AuthorizationOverviewOrganizationFact, len(units))
	ids := make([]uint, len(units))
	for index, unit := range units {
		facts[index] = authapplication.AuthorizationOverviewOrganizationFact{ID: unit.ID, Name: unit.Name, Manageable: true}
		ids[index] = unit.ID
	}
	return facts, ids, nil
}

func uniqueOverviewIDs(ids []uint) []uint {
	result := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
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
