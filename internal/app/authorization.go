package app

import (
	"context"

	authgorm "admin/internal/authorization/adapters/gorm"
	authhttp "admin/internal/authorization/adapters/http"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	"admin/internal/identity"
	"admin/internal/organization"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"

	"gorm.io/gorm"
)

func newAuthorizationService(resources Resources) *authapplication.Service {
	repository := authgorm.NewRepository(resources.DB)
	organizationRepository := organization.NewGORMRepository(resources.DB)
	hierarchy := organization.NewHierarchy(organizationRepository)
	organizations := organization.NewScopeReader(organizationRepository, hierarchy)
	users := identityAuthorizationUsers{db: resources.DB}
	versions := authgorm.NewAccessVersions(resources.DB)
	return authapplication.NewService(
		repository,
		platformdatabase.NewTransactionRunner(resources.DB),
		organizations,
		users,
		versions,
	)
}

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

type identityAuthorizationUsers struct{ db *gorm.DB }

func (adapter identityAuthorizationUsers) ListUsersByIDs(ctx context.Context, userIDs []uint) ([]authdomain.UserSummary, error) {
	if len(userIDs) == 0 {
		return []authdomain.UserSummary{}, nil
	}
	var users []identity.User
	if err := platformdatabase.FromContext(ctx, adapter.db).Where("id IN ? AND deleted_at IS NULL", userIDs).Order("id asc").Find(&users).Error; err != nil {
		return nil, err
	}
	result := make([]authdomain.UserSummary, len(users))
	for index, user := range users {
		result[index] = authdomain.UserSummary{
			ID: user.ID, Username: user.Username, Nickname: user.Nickname, Avatar: user.Avatar,
			Email: user.Email, Role: user.Role, Status: user.Status,
		}
	}
	return result, nil
}

var _ authapplication.UserDirectory = identityAuthorizationUsers{}

type authorizationOrganizationVisibility struct {
	authorization *authapplication.Service
}

func (visibility authorizationOrganizationVisibility) Scope(ctx context.Context, operatorID uint) (organization.OrganizationScope, error) {
	scope, err := visibility.authorization.ResolveOrganizationScope(ctx, authdomain.Principal{UserID: operatorID})
	return organization.OrganizationScope{All: scope.All, OrganizationIDs: scope.OrganizationIDs}, err
}
