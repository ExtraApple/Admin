package app

import (
	"context"
	"strings"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	apiapplication "admin/internal/apimetadata/application"
	apidomain "admin/internal/apimetadata/domain"
	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
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
	transactions := platformdatabase.NewTransactionRunner(resources.DB)
	apiCore := apiapplication.NewCore(apigorm.NewRepository(resources.DB), transactions, authorizationCapability)
	navigationService := navigation.NewService(
		navigation.NewGORMRepository(resources.DB),
		transactions,
		authorizationCapability,
		navigationAPIStorage{core: apiCore},
	)
	coordinator := apiNavigationCoordinator{navigation: navigationService}
	apiService := apiapplication.NewService(apiCore, coordinator, apiapplication.WithRouteSource(routeSource))
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
func (storage navigationAPIStorage) ListByPermissionCode(ctx context.Context, code string) ([]navigation.APIRecord, error) {
	apis, err := storage.core.ListByPermissionCode(ctx, code)
	return mapAPIsToNavigation(apis), err
}
func (storage navigationAPIStorage) LockDependencies(ctx context.Context, ids []uint, codes []string) ([]navigation.APIRecord, error) {
	apis, err := storage.core.LockDependencies(ctx, ids, codes)
	return mapAPIsToNavigation(apis), err
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
	return navigation.APIRecord{ID: api.ID, Name: api.Name, Method: api.Method, Path: api.Path, Group: api.Group, PermissionCode: api.PermissionCode, Sort: api.Sort, Status: api.Status, NeedAuth: api.NeedAuth, NeedAudit: api.NeedAudit, Remark: api.Remark, Deleted: api.Deleted}
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

func (capability navigationAuthorizationCapability) RoleConfiguration(ctx context.Context, roleID uint) (navigation.RoleConfigurationFact, error) {
	if roleID == 0 {
		return navigation.RoleConfigurationFact{}, nil
	}
	role, err := capability.repository.FindRole(ctx, roleID)
	if err == authapplication.ErrNotFound {
		return navigation.RoleConfigurationFact{}, nil
	}
	if err != nil {
		return navigation.RoleConfigurationFact{}, err
	}
	return navigation.RoleConfigurationFact{Exists: true, Protected: authdomain.IsProtectedRole(role.Code)}, nil
}
func (capability navigationAuthorizationCapability) LockRoleConfiguration(ctx context.Context, roleID uint) (navigation.RoleConfigurationFact, error) {
	if roleID == 0 {
		return navigation.RoleConfigurationFact{}, nil
	}
	role, err := capability.repository.LockRole(ctx, roleID)
	if err == authapplication.ErrNotFound {
		return navigation.RoleConfigurationFact{}, nil
	}
	if err != nil {
		return navigation.RoleConfigurationFact{}, err
	}
	return navigation.RoleConfigurationFact{Exists: true, Protected: authdomain.IsProtectedRole(role.Code)}, nil
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
func (capability navigationAuthorizationCapability) LockPermissions(ctx context.Context, codes []string) error {
	return capability.repository.LockPermissionsByCodes(ctx, codes)
}
func (capability navigationAuthorizationCapability) ExistingPermissionCodes(ctx context.Context, codes []string, includeDeleted bool) ([]string, error) {
	return capability.repository.ExistingPermissionCodes(ctx, codes, includeDeleted)
}
func (capability navigationAuthorizationCapability) LockAccessVersions(ctx context.Context, userIDs []uint) error {
	return capability.versions.LockExisting(ctx, userIDs)
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

func (coordinator apiNavigationCoordinator) WithAPIPermissionSync(ctx context.Context, apis []apidomain.API, work func(context.Context) error) error {
	targets := make([]navigation.APIPermissionSyncTarget, 0, len(apis))
	for _, api := range apis {
		if api.NeedAuth != 1 {
			continue
		}
		code := api.PermissionCode
		if strings.TrimSpace(code) == "" {
			code = apiapplication.DerivePermissionCode(api.Method, api.Path)
		}
		targets = append(targets, navigation.APIPermissionSyncTarget{API: mapAPIToNavigation(api), Code: code})
	}
	return apiCoordinatorError(coordinator.navigation.WithAPIPermissionSync(ctx, targets, work))
}

func (coordinator apiNavigationCoordinator) ExistingPermissionCodes(ctx context.Context, codes []string, includeDeleted bool) ([]string, error) {
	return coordinator.navigation.ExistingPermissionCodes(ctx, codes, includeDeleted)
}

func (coordinator apiNavigationCoordinator) ChangeAPIPermissionCode(ctx context.Context, apiID uint, code string, update func(context.Context) error) error {
	return apiCoordinatorError(coordinator.navigation.ChangeAPIPermissionCode(ctx, apiID, code, update))
}
func (coordinator apiNavigationCoordinator) DeleteAPI(ctx context.Context, apiID uint, deleteAPI func(context.Context) error) error {
	return apiCoordinatorError(coordinator.navigation.DeleteAPI(ctx, apiID, deleteAPI))
}

func apiCoordinatorError(err error) error {
	if code, ok := navigation.CodeOf(err); ok && code == navigation.CodeConflict {
		return apiapplication.NewError(apiapplication.CodeConflict, err)
	}
	return err
}
func (coordinator apiNavigationCoordinator) EnsureAPIPermission(ctx context.Context, api apidomain.API) (bool, error) {
	return coordinator.navigation.EnsureAPIPermission(ctx, mapAPIToNavigation(api))
}
func (coordinator apiNavigationCoordinator) GenerateMenuButton(ctx context.Context, apiID, parentID uint, name string, sort int) (apiapplication.Button, error) {
	menu, err := coordinator.navigation.GenerateMenuButton(ctx, apiID, parentID, name, sort)
	return apiapplication.Button{ID: menu.ID, ParentID: menu.ParentID, Name: menu.Name, Path: menu.Path, Component: menu.Component, Icon: menu.Icon, PermissionCode: menu.PermissionCode, Sort: menu.Sort, Type: menu.Type, Status: menu.Status}, err
}

var _ apiapplication.PermissionCoordinator = apiNavigationCoordinator{}
