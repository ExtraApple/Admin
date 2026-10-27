package app

import (
	"context"

	authgorm "admin/internal/authorization/adapters/gorm"
	authhttp "admin/internal/authorization/adapters/http"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	identityapplication "admin/internal/identity/application"
	"admin/internal/navigation"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
)

func newAuthorizationService(resources Resources, users authapplication.OverviewUserDirectory, managedUsers identityapplication.UserManagementReader) *authapplication.Service {
	repository := authgorm.NewRepository(resources.DB)
	organizationRepository := organization.NewGORMRepository(resources.DB)
	hierarchy := organization.NewHierarchy(organizationRepository)
	organizations := organization.NewScopeReader(organizationRepository, hierarchy)
	versions := authgorm.NewAccessVersions(resources.DB)
	overview := authorizationOverviewReader{
		authorization: repository,
		navigation:    navigation.NewGORMRepository(resources.DB),
		organizations: organizationRepository,
		users:         users,
	}
	return authapplication.NewService(
		repository,
		platformdatabase.NewTransactionRunner(resources.DB),
		organizations,
		users,
		versions,
		overview,
		authorizationManagedUsers{reader: managedUsers},
		navigation.NewGORMRepository(resources.DB),
	)
}

type authorizationManagedUsers struct {
	reader identityapplication.UserManagementReader
}

func (adapter authorizationManagedUsers) ManagementUsersByIDs(ctx context.Context, userIDs []uint) ([]authapplication.ManagedUserFact, error) {
	records, err := adapter.reader.ManagementUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	facts := make([]authapplication.ManagedUserFact, len(records))
	for index, record := range records {
		facts[index] = authapplication.ManagedUserFact{ID: record.ID, Deleted: record.Deleted}
	}
	return facts, nil
}

var _ authapplication.UserDirectory = (*identityapplication.DirectoryService)(nil)

func authorizationDescriptors(application *authapplication.Service, existing []routecatalog.Descriptor) []routecatalog.Descriptor {
	initial := authhttp.Routes(application, nil)
	facts := routeFacts(existing)
	facts = append(facts, routeFacts(initial)...)
	return authhttp.Routes(application, authorizationRouteSource{facts: facts})
}

type authorizationRouteSource struct {
	facts []authdomain.RouteFact
}

func (source authorizationRouteSource) Routes(context.Context) ([]authdomain.RouteFact, error) {
	return append([]authdomain.RouteFact(nil), source.facts...), nil
}

func routeFacts(descriptors []routecatalog.Descriptor) []authdomain.RouteFact {
	facts := make([]authdomain.RouteFact, 0, len(descriptors))
	for index := range descriptors {
		descriptor := &descriptors[index]
		if descriptor.Access != routecatalog.PermissionControlled {
			continue
		}
		facts = append(facts, authdomain.RouteFact{
			Method:         descriptor.Method,
			Path:           descriptor.Path,
			PermissionCode: descriptor.DefaultPermissionCode,
		})
	}
	return facts
}

type authorizationOrganizationVisibility struct {
	authorization *authapplication.Service
}

func (visibility authorizationOrganizationVisibility) Scope(ctx context.Context, operatorID uint) (organization.OrganizationScope, error) {
	scope, err := visibility.authorization.ResolveOrganizationScope(ctx, authdomain.Principal{UserID: operatorID})
	return organization.OrganizationScope{All: scope.All, OrganizationIDs: scope.OrganizationIDs}, err
}

func (visibility authorizationOrganizationVisibility) CurrentScope(ctx context.Context, operatorID uint) (organization.OrganizationScope, error) {
	if _, err := visibility.authorization.ProtectedUserIDs(ctx, []uint{operatorID}); err != nil {
		return organization.OrganizationScope{}, err
	}
	scope, err := visibility.authorization.ResolveOrganizationManagementScope(ctx, authdomain.Principal{UserID: operatorID})
	return organization.OrganizationScope{All: scope.All, OrganizationIDs: scope.OrganizationIDs}, err
}
