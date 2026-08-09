package app

import (
	"context"

	authgorm "admin/internal/authorization/adapters/gorm"
	authhttp "admin/internal/authorization/adapters/http"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	identityapplication "admin/internal/identity/application"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
)

func newAuthorizationService(resources Resources, users authapplication.UserDirectory) *authapplication.Service {
	repository := authgorm.NewRepository(resources.DB)
	organizationRepository := organization.NewGORMRepository(resources.DB)
	hierarchy := organization.NewHierarchy(organizationRepository)
	organizations := organization.NewScopeReader(organizationRepository, hierarchy)
	versions := authgorm.NewAccessVersions(resources.DB)
	return authapplication.NewService(
		repository,
		platformdatabase.NewTransactionRunner(resources.DB),
		organizations,
		users,
		versions,
	)
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
	facts := make([]authdomain.RouteFact, len(descriptors))
	for index, descriptor := range descriptors {
		facts[index] = authdomain.RouteFact{
			Method:         descriptor.Method,
			Path:           descriptor.Path,
			PermissionCode: descriptor.DefaultPermissionCode,
			Public:         descriptor.Access == routecatalog.Public,
		}
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
