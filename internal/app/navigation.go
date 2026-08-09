package app

import (
	"context"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	apiapplication "admin/internal/apimetadata/application"
	apidomain "admin/internal/apimetadata/domain"
	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	identityapplication "admin/internal/identity/application"
	"admin/internal/navigation"
	platformdatabase "admin/internal/platform/database"
)

type navigationComposition struct {
	Navigation *navigation.Service
	APICore    *apiapplication.Core
	APIService *apiapplication.Service
}

func newNavigationComposition(resources Resources, authorization *authapplication.Service, routeSource apiapplication.RouteSource, users identityapplication.UserDirectory) navigationComposition {
	authorizationRepository := authgorm.NewRepository(resources.DB)
	versions := authgorm.NewAccessVersions(resources.DB)
	authorizationCapability := navigationAuthorizationCapability{
		authorization: authorization, repository: authorizationRepository,
		versions: versions, users: users,
	}
	apiCore := apiapplication.NewCore(apigorm.NewRepository(resources.DB))
	transactions := platformdatabase.NewTransactionRunner(resources.DB)
	navigationService := navigation.NewService(
		navigation.NewGORMRepository(resources.DB),
		transactions,
		authorizationCapability,
		navigationAPIStorage{core: apiCore},
	)
	coordinator := apiNavigationCoordinator{navigation: navigationService}
	apiService := apiapplication.NewService(apiCore, coordinator, apiapplication.WithRouteSource(transactions, routeSource))
	return navigationComposition{Navigation: navigationService, APICore: apiCore, APIService: apiService}
}

type navigationAPIStorage struct{ core *apiapplication.Core }

func (storage navigationAPIStorage) ListByIDs(ctx context.Context, ids []uint) ([]navigation.APIRecord, error) {
	apis, err := storage.core.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return mapAPIsToNavigation(apis), nil
}
func (storage navigationAPIStorage) Lock(ctx context.Context, id uint) (navigation.APIRecord, error) {
	api, err := storage.core.Lock(ctx, id)
	if err != nil {
		return navigation.APIRecord{}, err
	}
	return mapAPIToNavigation(api), nil
}
func (storage navigationAPIStorage) LockMany(ctx context.Context, ids []uint) ([]navigation.APIRecord, error) {
	apis, err := storage.core.LockMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	return mapAPIsToNavigation(apis), nil
}
func (storage navigationAPIStorage) SetPermissionCode(ctx context.Context, ids []uint, code string) error {
	return storage.core.SetPermissionCode(ctx, ids, code)
}
func (storage navigationAPIStorage) CountPermissionCode(ctx context.Context, code string) (int64, error) {
	return storage.core.CountPermissionCode(ctx, code)
}

func mapAPIToNavigation(api apidomain.API) navigation.APIRecord {
	return navigation.APIRecord{ID: api.ID, Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group, PermissionCode: api.PermissionCode, Sort: api.Sort, Status: api.Status, NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit, Remark: api.Remark}
}
func mapAPIsToNavigation(apis []apidomain.API) []navigation.APIRecord {
	result := make([]navigation.APIRecord, len(apis))
	for index := range apis {
		result[index] = mapAPIToNavigation(apis[index])
	}
	return result
}

type navigationAuthorizationCapability struct {
	authorization *authapplication.Service
	repository    *authgorm.Repository
	versions      *authgorm.AccessVersions
	users         identityapplication.UserDirectory
}

func (capability navigationAuthorizationCapability) RoleExists(ctx context.Context, roleID uint) (bool, error) {
	_, err := capability.repository.FindRole(ctx, roleID)
	if err == authapplication.ErrNotFound {
		return false, nil
	}
	return err == nil, err
}
func (capability navigationAuthorizationCapability) UserAccess(ctx context.Context, userID uint) (navigation.UserAccess, error) {
	roles, err := capability.repository.RolesForUser(ctx, userID)
	if err != nil {
		return navigation.UserAccess{}, err
	}
	permissions, err := capability.authorization.Permissions(ctx, userID)
	if err != nil {
		return navigation.UserAccess{}, err
	}
	roleIDs := make([]uint, len(roles))
	isAdmin := false
	for index, role := range roles {
		roleIDs[index] = role.ID
		if role.Code == "admin" {
			isAdmin = true
		}
	}
	return navigation.UserAccess{RoleIDs: roleIDs, IsAdmin: isAdmin, Permissions: permissions}, nil
}
func (capability navigationAuthorizationCapability) UserIDsByRoleIDs(ctx context.Context, roleIDs []uint) ([]uint, error) {
	return capability.repository.UserIDsByRoleIDs(ctx, roleIDs)
}
func (capability navigationAuthorizationCapability) AllUserIDs(ctx context.Context) ([]uint, error) {
	return capability.users.ListUserIDs(ctx)
}
func (capability navigationAuthorizationCapability) RoleIDsByPermissionCode(ctx context.Context, code string) ([]uint, error) {
	return capability.repository.RoleIDsByPermissionCode(ctx, code)
}
func (capability navigationAuthorizationCapability) LockPermission(ctx context.Context, code string) (navigation.PermissionRef, bool, error) {
	permission, found, err := capability.repository.LockPermissionByCode(ctx, code)
	return navigation.PermissionRef{ID: permission.ID, Code: permission.Code.String()}, found, err
}
func (capability navigationAuthorizationCapability) EnsurePermission(ctx context.Context, seed navigation.PermissionSeed) (navigation.PermissionRef, bool, error) {
	permission, created, err := capability.repository.EnsurePermissionByCode(ctx, seed.Code, seed.Name, seed.Group, seed.Sort)
	return navigation.PermissionRef{ID: permission.ID, Code: permission.Code.String()}, created, err
}
func (capability navigationAuthorizationCapability) MergePermissionRoles(ctx context.Context, fromID, toID uint) error {
	return capability.repository.MergePermissionRoles(ctx, fromID, toID)
}
func (capability navigationAuthorizationCapability) PermissionHasRoleReferences(ctx context.Context, permissionID uint) (bool, error) {
	return capability.repository.PermissionHasRoleReferences(ctx, permissionID)
}
func (capability navigationAuthorizationCapability) DeletePermission(ctx context.Context, permissionID uint) error {
	return capability.repository.DeletePermissionForNavigation(ctx, permissionID)
}
func (capability navigationAuthorizationCapability) IncrementAccessVersions(ctx context.Context, userIDs []uint) error {
	return capability.versions.Increment(ctx, userIDs)
}

var _ navigation.Authorization = navigationAuthorizationCapability{}
var _ navigation.APIMetadataReader = navigationAPIStorage{}

type apiNavigationCoordinator struct{ navigation *navigation.Service }

func (coordinator apiNavigationCoordinator) ChangeAPIPermissionCode(ctx context.Context, apiID uint, code string, update func(context.Context) error) error {
	return coordinator.navigation.ChangeAPIPermissionCode(ctx, apiID, code, update)
}
func (coordinator apiNavigationCoordinator) DeleteAPI(ctx context.Context, apiID uint, deleteAPI func(context.Context) error) error {
	return coordinator.navigation.DeleteAPI(ctx, apiID, deleteAPI)
}
func (coordinator apiNavigationCoordinator) EnsureAPIPermission(ctx context.Context, api apidomain.API) (bool, error) {
	return coordinator.navigation.EnsureAPIPermission(ctx, mapAPIToNavigation(api))
}
func (coordinator apiNavigationCoordinator) GenerateMenuButton(ctx context.Context, apiID, parentID uint, name string, sort int) (apiapplication.Button, error) {
	menu, err := coordinator.navigation.GenerateMenuButton(ctx, apiID, parentID, name, sort)
	return apiapplication.Button{ID: menu.ID, ParentID: menu.ParentID, Name: menu.Name, Path: menu.Path, Component: menu.Component, Icon: menu.Icon, PermissionCode: menu.PermissionCode, Sort: menu.Sort, Type: menu.Type, Status: menu.Status}, err
}

var _ apiapplication.PermissionCoordinator = apiNavigationCoordinator{}
