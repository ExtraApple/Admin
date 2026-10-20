package app

import (
	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	identityapplication "admin/internal/identity/application"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
	"context"
)

type organizationComposition struct {
	routes      []routecatalog.Descriptor
	memberships *organization.UserMembershipService
}

func newOrganizationComposition(resources Resources, visibility organization.VisibilityProvider, users organization.UserDirectory, authorization *authapplication.Service, managedUsers identityapplication.UserManagementReader) organizationComposition {
	repository := organization.NewGORMRepository(resources.DB)
	hierarchy := organization.NewHierarchy(repository)
	versions := authgorm.NewAccessVersions(resources.DB)
	transactions := platformdatabase.NewTransactionRunner(resources.DB)
	policy := organizationUserPolicy{authorization: authorization, users: managedUsers}
	service := organization.NewService(repository, hierarchy, visibility, users, versions, transactions, policy, authorization, organizationHierarchyDependencies{authorization: authorization})
	memberships := organization.NewUserMembershipService(repository, visibility, policy, versions, transactions)
	routes := append(organization.Routes(service), organization.UserMembershipRoutes(memberships)...)
	return organizationComposition{routes: routes, memberships: memberships}
}

type organizationHierarchyDependencies struct {
	authorization *authapplication.Service
}

func (reader organizationHierarchyDependencies) HierarchyDependentUserIDs(ctx context.Context, roots []uint) ([]uint, error) {
	userIDs, err := reader.authorization.HierarchyDependentUserIDs(ctx, roots)
	if code, ok := authapplication.CodeOf(err); ok && code == authapplication.CodeConflict {
		return nil, organization.NewError(organization.CodeConflict, err)
	}
	return userIDs, err
}

type organizationUserPolicy struct {
	authorization *authapplication.Service
	users         identityapplication.UserManagementReader
}

func (policy organizationUserPolicy) ManagedUsers(ctx context.Context, operatorID uint, userIDs []uint) (map[uint]organization.ManagedUser, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	protected, err := policy.authorization.ProtectedUserIDs(ctx, append(append([]uint{}, userIDs...), operatorID))
	if err != nil {
		return nil, err
	}
	users, err := policy.users.ManagementUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	scope, err := policy.authorization.ResolveUserManagementScope(ctx, authdomain.Principal{UserID: operatorID})
	if err != nil {
		return nil, err
	}
	facts := make(map[uint]organization.ManagedUser, len(users))
	for _, user := range users {
		facts[user.ID] = organization.ManagedUser{Exists: true, Deleted: user.Deleted, Visible: scope.All}
	}
	for _, userID := range scope.UserIDs {
		if user, exists := facts[userID]; exists {
			user.Visible = true
			facts[userID] = user
		}
	}
	for _, userID := range protected {
		if user, exists := facts[userID]; exists {
			user.Protected = true
			facts[userID] = user
		}
	}
	return facts, nil
}

type identityOrganizationReader struct {
	memberships *organization.UserMembershipService
}

func (reader identityOrganizationReader) UserOrganizations(ctx context.Context, operatorID uint, userIDs []uint) ([]identityapplication.UserOrganizationFact, error) {
	facts, err := reader.memberships.UserOrganizations(ctx, operatorID, userIDs)
	if err != nil {
		return nil, err
	}
	result := make([]identityapplication.UserOrganizationFact, len(facts))
	for index, fact := range facts {
		result[index] = identityapplication.UserOrganizationFact{UserID: fact.UserID, ID: fact.OrganizationID, Name: fact.Name, Manageable: fact.Manageable}
	}
	return result, nil
}

var _ identityapplication.AdminOrganizationReader = identityOrganizationReader{}

var _ organization.UserDirectory = (*identityapplication.DirectoryService)(nil)
