package application

import (
	"context"
	"errors"
	"strings"

	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
)

var (
	ErrInvalidUser            = NewError(CodeInvalidUser, nil)
	ErrPermissionCodeNotFound = NewError(CodePermissionCodeAbsent, nil)
)

type Service struct {
	repository    Repository
	transactions  TransactionRunner
	organizations OrganizationScopeReader
	users         UserDirectory
	versions      AccessVersionStore
}

func NewService(repository Repository, transactions TransactionRunner, organizations OrganizationScopeReader, users UserDirectory, versions AccessVersionStore) *Service {
	return &Service{repository: repository, transactions: transactions, organizations: organizations, users: users, versions: versions}
}

// EnsureAccessVersion initializes a user's authorization version without invalidating existing tokens.
func (service *Service) EnsureAccessVersion(ctx context.Context, userID uint) (int, error) {
	return service.versions.Ensure(ctx, userID)
}

type CreateRoleRequest struct {
	Name        string
	Code        string
	Description string
	Sort        int
	Status      int
	DataScope   string
}

type UpdateRoleRequest struct {
	Name        string
	Code        string
	Description string
	Sort        *int
	Status      *int
	DataScope   string
}

type RolePage struct {
	List  []domain.Role
	Total int64
	Page  int
	Size  int
}

func (service *Service) ListRoles(ctx context.Context, page, size int) (RolePage, error) {
	page, size = normalizePage(page, size)
	roles, total, err := service.repository.ListRoles(ctx, (page-1)*size, size)
	if err != nil {
		return RolePage{}, NewError(CodeInternalError, err)
	}
	return RolePage{List: roles, Total: total, Page: page, Size: size}, nil
}

func (service *Service) CreateRole(ctx context.Context, request CreateRoleRequest) (domain.Role, error) {
	scope, err := domain.ParseDataScope(request.DataScope)
	if err != nil {
		return domain.Role{}, NewError(CodeValidationInvalid, err)
	}
	if scope == domain.DataScopeCustom {
		return domain.Role{}, NewError(CodeValidationInvalid, nil)
	}
	nameExists, err := service.repository.RoleNameExists(ctx, request.Name, 0)
	if err != nil {
		return domain.Role{}, wrapError(err)
	}
	codeExists, err := service.repository.RoleCodeExists(ctx, request.Code, 0)
	if err != nil {
		return domain.Role{}, wrapError(err)
	}
	if nameExists || codeExists {
		return domain.Role{}, NewError(CodeConflict, nil)
	}
	status := request.Status
	if status != 0 && status != 1 {
		status = 1
	}
	role := domain.Role{Name: request.Name, Code: request.Code, Description: request.Description, Sort: request.Sort, Status: status, DataScope: scope}
	if err := service.repository.CreateRole(ctx, &role); err != nil {
		return domain.Role{}, NewError(CodeInternalError, err)
	}
	return role, nil
}

func (service *Service) UpdateRole(ctx context.Context, roleID uint, request UpdateRoleRequest) (domain.Role, error) {
	role, err := service.repository.FindRole(ctx, roleID)
	if err != nil {
		return domain.Role{}, roleError(err)
	}
	if domain.IsProtectedRole(role.Code) {
		return domain.Role{}, NewError(CodeConflict, nil)
	}
	if request.Name != "" {
		role.Name = request.Name
	}
	if request.Code != "" {
		role.Code = request.Code
	}
	if request.Description != "" {
		role.Description = request.Description
	}
	if request.Sort != nil {
		role.Sort = *request.Sort
	}
	if request.Status != nil {
		role.Status = *request.Status
	}
	if request.DataScope != "" {
		role.DataScope, err = domain.ParseDataScope(request.DataScope)
		if err != nil {
			return domain.Role{}, NewError(CodeValidationInvalid, err)
		}
		if role.DataScope == domain.DataScopeCustom {
			return domain.Role{}, NewError(CodeValidationInvalid, nil)
		}
	}
	if request.Name == "" && request.Code == "" && request.Description == "" && request.Sort == nil && request.Status == nil && request.DataScope == "" {
		return domain.Role{}, NewError(CodeValidationInvalid, nil)
	}
	if request.Name != "" {
		exists, err := service.repository.RoleNameExists(ctx, request.Name, roleID)
		if err != nil {
			return domain.Role{}, wrapError(err)
		}
		if exists {
			return domain.Role{}, NewError(CodeConflict, nil)
		}
	}
	if request.Code != "" {
		exists, err := service.repository.RoleCodeExists(ctx, request.Code, roleID)
		if err != nil {
			return domain.Role{}, wrapError(err)
		}
		if exists {
			return domain.Role{}, NewError(CodeConflict, nil)
		}
	}
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		if err := service.repository.UpdateRole(tx, role); err != nil {
			return err
		}
		return service.incrementRoleUsers(tx, roleID)
	}); err != nil {
		return domain.Role{}, NewError(CodeInternalError, err)
	}
	return service.repository.FindRole(ctx, roleID)
}

func (service *Service) DeleteRole(ctx context.Context, roleID uint) error {
	role, err := service.repository.FindRole(ctx, roleID)
	if err != nil {
		return roleError(err)
	}
	if domain.IsProtectedRole(role.Code) {
		return NewError(CodeConflict, nil)
	}
	return service.transactions.Run(ctx, func(tx context.Context) error {
		userIDs, err := service.repository.UserIDsByRole(tx, roleID)
		if err != nil {
			return err
		}
		if err := service.repository.DeleteRole(tx, roleID); err != nil {
			return err
		}
		return service.incrementUsers(tx, userIDs)
	})
}

func (service *Service) AssignUsersToRole(ctx context.Context, roleID uint, userIDs []uint) error {
	if _, err := service.repository.FindRole(ctx, roleID); err != nil {
		return NewError(CodeNotFound, err)
	}
	return service.transactions.Run(ctx, func(tx context.Context) error {
		oldIDs, err := service.repository.UserIDsByRole(tx, roleID)
		if err != nil {
			return err
		}
		if err := service.repository.ReplaceRoleUsers(tx, roleID, userIDs); err != nil {
			return NewError(CodeInternalError, err)
		}
		return service.incrementUsers(tx, append(oldIDs, userIDs...))
	})
}

func (service *Service) RoleUsers(ctx context.Context, roleID uint) ([]identitydomain.DirectoryUser, error) {
	if _, err := service.repository.FindRole(ctx, roleID); err != nil {
		return nil, NewError(CodeNotFound, err)
	}
	ids, err := service.repository.UserIDsByRole(ctx, roleID)
	if err != nil {
		return nil, wrapError(err)
	}
	users, err := service.users.ListUsersByIDs(ctx, ids)
	if err != nil {
		return nil, wrapError(err)
	}
	if users == nil {
		return []identitydomain.DirectoryUser{}, nil
	}
	return users, nil
}

func (service *Service) AssignRoleDataScope(ctx context.Context, roleID uint, scopeValue string, organizationIDs []uint) error {
	scope, err := domain.ParseDataScope(scopeValue)
	if err != nil {
		return NewError(CodeValidationInvalid, err)
	}
	role, err := service.repository.FindRole(ctx, roleID)
	if err != nil {
		return NewError(CodeNotFound, err)
	}
	if domain.IsProtectedRole(role.Code) {
		return NewError(CodeConflict, nil)
	}
	organizationIDs = uniqueIDs(organizationIDs)
	if scope == domain.DataScopeCustom {
		if len(organizationIDs) == 0 {
			return NewError(CodeValidationInvalid, nil)
		}
		existing, err := service.organizations.ExistingOrganizationIDs(ctx, organizationIDs)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		if !sameIDs(existing, organizationIDs) {
			return NewError(CodeValidationInvalid, nil)
		}
	}
	return service.transactions.Run(ctx, func(tx context.Context) error {
		if err := service.repository.ReplaceRoleDataScope(tx, roleID, scope, organizationIDs); err != nil {
			return err
		}
		return service.incrementRoleUsers(tx, roleID)
	})
}

func (service *Service) RoleDataScope(ctx context.Context, roleID uint) (domain.RoleDataScope, error) {
	if _, err := service.repository.FindRole(ctx, roleID); err != nil {
		return domain.RoleDataScope{}, NewError(CodeNotFound, err)
	}
	return service.repository.RoleDataScope(ctx, roleID)
}

type CreatePermissionRequest struct {
	Name  string
	Code  string
	Group string
	Sort  int
}

type UpdatePermissionRequest struct {
	Name  string
	Group string
	Sort  *int
}

type PermissionPage struct {
	List  []domain.Permission
	Total int64
	Page  int
	Size  int
}

func (service *Service) ListPermissions(ctx context.Context, page, size int) (PermissionPage, error) {
	page, size = normalizePage(page, size)
	permissions, total, err := service.repository.ListPermissions(ctx, (page-1)*size, size)
	if err != nil {
		return PermissionPage{}, NewError(CodeInternalError, err)
	}
	return PermissionPage{List: permissions, Total: total, Page: page, Size: size}, nil
}

func (service *Service) CreatePermission(ctx context.Context, request CreatePermissionRequest) (domain.Permission, error) {
	code, err := domain.NewPermissionCode(request.Code)
	if err != nil {
		return domain.Permission{}, NewError(CodeValidationInvalid, err)
	}
	exists, err := service.repository.PermissionCodeExists(ctx, code.String())
	if err != nil {
		return domain.Permission{}, wrapError(err)
	}
	if exists {
		return domain.Permission{}, NewError(CodeConflict, nil)
	}
	permission := domain.Permission{Name: request.Name, Code: code, Group: request.Group, Sort: request.Sort}
	if err := service.repository.CreatePermission(ctx, &permission); err != nil {
		return domain.Permission{}, NewError(CodeInternalError, err)
	}
	return permission, nil
}

func (service *Service) UpdatePermission(ctx context.Context, permissionID uint, request UpdatePermissionRequest) (domain.Permission, error) {
	permission, err := service.repository.FindPermission(ctx, permissionID)
	if err != nil {
		return domain.Permission{}, permissionError(err)
	}
	if request.Name == "" && request.Group == "" && request.Sort == nil {
		return domain.Permission{}, NewError(CodeValidationInvalid, nil)
	}
	if request.Name != "" {
		permission.Name = request.Name
	}
	if request.Group != "" {
		permission.Group = request.Group
	}
	if request.Sort != nil {
		permission.Sort = *request.Sort
	}
	if err := service.repository.UpdatePermission(ctx, permission); err != nil {
		return domain.Permission{}, NewError(CodeInternalError, err)
	}
	return service.repository.FindPermission(ctx, permissionID)
}

func (service *Service) DeletePermission(ctx context.Context, permissionID uint) error {
	if _, err := service.repository.FindPermission(ctx, permissionID); err != nil {
		return permissionError(err)
	}
	return service.transactions.Run(ctx, func(tx context.Context) error {
		roleIDs, err := service.repository.RoleIDsByPermission(tx, permissionID)
		if err != nil {
			return err
		}
		if err := service.repository.DeletePermission(tx, permissionID); err != nil {
			return err
		}
		return service.incrementRoles(tx, roleIDs)
	})
}

func (service *Service) AssignPermissionsToRole(ctx context.Context, roleID uint, permissionIDs []uint) error {
	if _, err := service.repository.FindRole(ctx, roleID); err != nil {
		return NewError(CodeNotFound, err)
	}
	return service.transactions.Run(ctx, func(tx context.Context) error {
		if err := service.repository.ReplaceRolePermissions(tx, roleID, permissionIDs); err != nil {
			return NewError(CodeInternalError, err)
		}
		return service.incrementRoleUsers(tx, roleID)
	})
}

func (service *Service) RolePermissions(ctx context.Context, roleID uint) ([]domain.Permission, error) {
	if _, err := service.repository.FindRole(ctx, roleID); err != nil {
		return nil, NewError(CodeNotFound, err)
	}
	permissions, err := service.repository.PermissionsByRole(ctx, roleID)
	if err != nil {
		return nil, wrapError(err)
	}
	if permissions == nil {
		return []domain.Permission{}, nil
	}
	return permissions, nil
}

func (service *Service) Permissions(ctx context.Context, userID uint) ([]string, error) {
	return service.repository.PermissionCodesForUser(ctx, userID)
}

func (service *Service) AllPermissionCodes(ctx context.Context) ([]string, error) {
	return service.repository.AllPermissionCodes(ctx)
}

func (service *Service) Snapshot(ctx context.Context, principal domain.Principal) (domain.AccessSnapshot, error) {
	if principal.UserID == 0 {
		return domain.AccessSnapshot{}, ErrInvalidUser
	}
	roles, err := service.repository.RolesForUser(ctx, principal.UserID)
	if err != nil {
		return domain.AccessSnapshot{}, wrapError(err)
	}
	permissions, err := service.repository.PermissionCodesForUser(ctx, principal.UserID)
	if err != nil {
		return domain.AccessSnapshot{}, wrapError(err)
	}
	version, err := service.versions.Current(ctx, principal.UserID)
	if err != nil {
		return domain.AccessSnapshot{}, wrapError(err)
	}
	roleCodes := make([]string, len(roles))
	for index := range roles {
		roleCodes[index] = roles[index].Code
	}
	return domain.AccessSnapshot{Principal: principal, Roles: roleCodes, Permissions: permissions, Version: version}, nil
}

func (service *Service) ResolveUserScope(ctx context.Context, principal domain.Principal) (domain.UserScope, error) {
	organizationScope, err := service.ResolveOrganizationScope(ctx, principal)
	if err != nil {
		return domain.UserScope{}, wrapError(err)
	}
	if organizationScope.All {
		return domain.AllUsersScope(), nil
	}
	userIDs := []uint{principal.UserID}
	for _, organizationID := range organizationScope.OrganizationIDs {
		ids, err := service.organizations.MemberUserIDs(ctx, organizationID)
		if err != nil {
			return domain.UserScope{}, NewError(CodeInternalError, err)
		}
		userIDs = append(userIDs, ids...)
	}
	return domain.UserScopeForIDs(userIDs), nil
}

func (service *Service) ResolveOrganizationScope(ctx context.Context, principal domain.Principal) (domain.OrganizationScope, error) {
	if principal.UserID == 0 {
		return domain.OrganizationScope{}, ErrInvalidUser
	}
	all, ids, err := service.scopeOrganizations(ctx, principal.UserID)
	if err != nil {
		return domain.OrganizationScope{}, wrapError(err)
	}
	if all {
		return domain.AllOrganizationsScope(), nil
	}
	return domain.OrganizationScopeForIDs(ids), nil
}

func (service *Service) VisibleUserIDs(ctx context.Context, operatorID uint) ([]uint, bool, error) {
	scope, err := service.ResolveUserScope(ctx, domain.Principal{UserID: operatorID})
	return scope.UserIDs, scope.All, err
}

func (service *Service) VisibleOrganizationIDs(ctx context.Context, operatorID uint) ([]uint, bool, error) {
	scope, err := service.ResolveOrganizationScope(ctx, domain.Principal{UserID: operatorID})
	return scope.OrganizationIDs, scope.All, err
}

func (service *Service) scopeOrganizations(ctx context.Context, operatorID uint) (bool, []uint, error) {
	roles, err := service.repository.RolesForUser(ctx, operatorID)
	if err != nil {
		return false, nil, NewError(CodeInternalError, err)
	}
	organizationIDs := make([]uint, 0)
	for _, role := range roles {
		if domain.IsProtectedRole(role.Code) || role.DataScope == domain.DataScopeAll {
			return true, nil, nil
		}
		switch role.DataScope {
		case domain.DataScopeSelf:
		case domain.DataScopeOrg:
			ids, err := service.organizations.MemberOrganizationIDs(ctx, operatorID)
			if err != nil {
				return false, nil, wrapError(err)
			}
			organizationIDs = append(organizationIDs, ids...)
		case domain.DataScopeOrgAndChildren:
			ids, err := service.organizations.MemberOrganizationIDs(ctx, operatorID)
			if err != nil {
				return false, nil, wrapError(err)
			}
			ids, err = service.organizations.DescendantOrganizationIDs(ctx, ids)
			if err != nil {
				return false, nil, wrapError(err)
			}
			organizationIDs = append(organizationIDs, ids...)
		case domain.DataScopeCustom:
			ids, err := service.repository.CustomOrganizationIDs(ctx, role.ID)
			if err != nil {
				return false, nil, wrapError(err)
			}
			organizationIDs = append(organizationIDs, ids...)
		}
	}
	return false, uniqueIDs(organizationIDs), nil
}

func (service *Service) incrementRoleUsers(ctx context.Context, roleID uint) error {
	ids, err := service.repository.UserIDsByRole(ctx, roleID)
	if err != nil {
		return wrapError(err)
	}
	return service.incrementUsers(ctx, ids)
}

func (service *Service) incrementRoles(ctx context.Context, roleIDs []uint) error {
	ids := make([]uint, 0)
	for _, roleID := range uniqueIDs(roleIDs) {
		userIDs, err := service.repository.UserIDsByRole(ctx, roleID)
		if err != nil {
			return wrapError(err)
		}
		ids = append(ids, userIDs...)
	}
	return service.incrementUsers(ctx, ids)
}

func (service *Service) incrementUsers(ctx context.Context, userIDs []uint) error {
	if service.versions == nil {
		return nil
	}
	for _, userID := range uniqueIDs(userIDs) {
		if _, err := service.versions.EnsureAndIncrement(ctx, userID); err != nil {
			return wrapError(err)
		}
	}
	return nil
}

func (service *Service) ListPermissionGroups(ctx context.Context, page, size int) (PermissionGroupPage, error) {
	page, size = normalizePage(page, size)
	groups, total, err := service.repository.ListPermissionGroups(ctx, (page-1)*size, size)
	if err != nil {
		return PermissionGroupPage{}, NewError(CodeInternalError, err)
	}
	return PermissionGroupPage{List: groups, Total: total, Page: page, Size: size}, nil
}

type PermissionGroupPage struct {
	List  []domain.PermissionGroup
	Total int64
	Page  int
	Size  int
}

func (service *Service) CreatePermissionGroup(ctx context.Context, name string, sort int) (domain.PermissionGroup, error) {
	exists, err := service.repository.PermissionGroupNameExists(ctx, name, 0)
	if err != nil {
		return domain.PermissionGroup{}, wrapError(err)
	}
	if exists {
		return domain.PermissionGroup{}, NewError(CodeConflict, nil)
	}
	group := domain.PermissionGroup{Name: name, Sort: sort}
	if err := service.repository.CreatePermissionGroup(ctx, &group); err != nil {
		return domain.PermissionGroup{}, NewError(CodeInternalError, err)
	}
	return group, nil
}

func (service *Service) UpdatePermissionGroup(ctx context.Context, groupID uint, name string, sort *int) (domain.PermissionGroup, error) {
	group, err := service.repository.FindPermissionGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return domain.PermissionGroup{}, NewError(CodeNotFound, err)
		}
		return domain.PermissionGroup{}, NewError(CodeInternalError, err)
	}
	if name == "" && sort == nil {
		return domain.PermissionGroup{}, NewError(CodeValidationInvalid, nil)
	}
	if name != "" {
		exists, err := service.repository.PermissionGroupNameExists(ctx, name, groupID)
		if err != nil {
			return domain.PermissionGroup{}, wrapError(err)
		}
		if exists {
			return domain.PermissionGroup{}, NewError(CodeConflict, nil)
		}
		group.Name = name
	}
	if sort != nil {
		group.Sort = *sort
	}
	if err := service.repository.UpdatePermissionGroup(ctx, group); err != nil {
		return domain.PermissionGroup{}, NewError(CodeInternalError, err)
	}
	return service.repository.FindPermissionGroup(ctx, groupID)
}

func (service *Service) DeletePermissionGroup(ctx context.Context, groupID uint) error {
	if _, err := service.repository.FindPermissionGroup(ctx, groupID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return NewError(CodeNotFound, err)
		}
		return NewError(CodeInternalError, err)
	}
	if err := service.repository.DeletePermissionGroup(ctx, groupID); err != nil {
		return NewError(CodeInternalError, err)
	}
	return nil
}

type PermissionSyncResult struct{ Created []string }

func (service *Service) SyncPermissions(ctx context.Context, routes []domain.RouteFact) (PermissionSyncResult, error) {
	result := PermissionSyncResult{Created: []string{}}
	err := service.transactions.Run(ctx, func(tx context.Context) error {
		for _, route := range routes {
			if route.Public {
				continue
			}
			code := route.PermissionCode
			if code == "" {
				code = generatedPermissionCode(route.Method, route.Path)
			}
			permissionCode, err := domain.NewPermissionCode(code)
			if err != nil {
				return err
			}
			exists, err := service.repository.PermissionCodeExists(tx, permissionCode.String())
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			group := permissionCode.String()
			if index := strings.IndexByte(group, '.'); index >= 0 {
				group = group[:index]
			}
			permission := domain.Permission{Name: route.Method + " " + route.Path, Code: permissionCode, Group: group}
			if err := service.repository.CreatePermission(tx, &permission); err != nil {
				return err
			}
			result.Created = append(result.Created, permissionCode.String())
		}
		return nil
	})
	return result, err
}

func generatedPermissionCode(method, path string) string {
	path = strings.TrimPrefix(path, "/api/")
	path = strings.ReplaceAll(path, ":", "")
	path = strings.ReplaceAll(path, "/", ".")
	return path + "." + strings.ToLower(method)
}

func normalizePage(page, size int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	return page, size
}

func roleError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return NewError(CodeNotFound, err)
	}
	return NewError(CodeInternalError, err)
}

func permissionError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return NewError(CodeNotFound, err)
	}
	return NewError(CodeInternalError, err)
}

func uniqueIDs(ids []uint) []uint {
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

func sameIDs(left, right []uint) bool {
	left, right = uniqueIDs(left), uniqueIDs(right)
	if len(left) != len(right) {
		return false
	}
	seen := make(map[uint]struct{}, len(left))
	for _, id := range left {
		seen[id] = struct{}{}
	}
	for _, id := range right {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

func containsID(ids []uint, target uint) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
