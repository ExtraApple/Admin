package app

import (
	"context"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	authgorm "admin/internal/authorization/adapters/gorm"
	identityapplication "admin/internal/identity/application"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
)

type organizationComposition struct {
	routes []routecatalog.Descriptor
	memberships *organization.UserMembershipService
}

func newOrganizationComposition(resources Resources, visibility organization.VisibilityProvider, users organization.UserDirectory, authorization *authapplication.Service) organizationComposition {
	repository := organization.NewGORMRepository(resources.DB)
	hierarchy := organization.NewHierarchy(repository)
	versions := authgorm.NewAccessVersions(resources.DB)
	transactions := platformdatabase.NewTransactionRunner(resources.DB)
	service := organization.NewService(repository, hierarchy, visibility, users, versions, transactions)
	memberships := organization.NewUserMembershipService(repository, visibility, organizationUserPolicy{authorization: authorization, users: users}, versions, transactions)
	routes := append(organization.Routes(service), organization.UserMembershipRoutes(memberships)...)
	return organizationComposition{routes: routes, memberships: memberships}
}

type organizationUserPolicy struct {
	authorization *authapplication.Service
	users organization.UserDirectory
}

func (policy organizationUserPolicy) ManagedUser(ctx context.Context, operatorID, targetID uint) (organization.ManagedUser, error) {
	users, err := policy.users.ListUsersByIDs(ctx, []uint{targetID})
	if err != nil || len(users) != 1 {
		return organization.ManagedUser{}, err
	}
	scope, err := policy.authorization.ResolveUserScope(ctx, authdomain.Principal{UserID: operatorID})
	if err != nil {
		return organization.ManagedUser{}, err
	}
	visible := scope.All
	for _, id := range scope.UserIDs {
		if id == targetID { visible = true; break }
	}
	roles, err := policy.authorization.UserRoleSummaries(ctx, []uint{targetID})
	if err != nil {
		return organization.ManagedUser{}, err
	}
	protected := false
	for _, role := range roles {
		if authdomain.IsProtectedRole(role.Code) { protected = true; break }
	}
	return organization.ManagedUser{Exists: true, Visible: visible, Protected: protected}, nil
}

type identityOrganizationReader struct{ memberships *organization.UserMembershipService }

func (reader identityOrganizationReader) UserOrganizations(ctx context.Context, operatorID uint, userIDs []uint) ([]identityapplication.UserOrganizationFact, error) {
	facts, err := reader.memberships.UserOrganizations(ctx, operatorID, userIDs)
	if err != nil { return nil, err }
	result := make([]identityapplication.UserOrganizationFact, len(facts))
	for index, fact := range facts {
		result[index] = identityapplication.UserOrganizationFact{UserID: fact.UserID, ID: fact.OrganizationID, Name: fact.Name, Manageable: fact.Manageable}
	}
	return result, nil
}

var _ identityapplication.AdminOrganizationReader = identityOrganizationReader{}

var _ organization.UserDirectory = (*identityapplication.DirectoryService)(nil)
