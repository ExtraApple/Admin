package application

import (
	"context"
	"errors"
	"slices"
	"sort"
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
	overview      AuthorizationOverviewReader
	managedUsers  UserManagementReader
	roleMenus     RoleMenuLifecycle
}

func NewService(repository Repository, transactions TransactionRunner, organizations OrganizationScopeReader, users UserDirectory, versions AccessVersionStore, overview AuthorizationOverviewReader, managedUsers UserManagementReader, roleMenus RoleMenuLifecycle) *Service {
	return &Service{repository: repository, transactions: transactions, organizations: organizations, users: users, versions: versions, overview: overview, managedUsers: managedUsers, roleMenus: roleMenus}
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
	Name        *string
	Code        *string
	Description *string
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
type UpdateUserRolesRequest struct {
	RoleIDs               []uint
	ExpectedAccessVersion int
}

func (service *Service) ListRoles(ctx context.Context, page, size int) (RolePage, error) {
	page, size = normalizePage(page, size)
	roles, total, err := service.repository.ListRoles(ctx, (page-1)*size, size)
	if err != nil {
		return RolePage{}, NewError(CodeInternalError, err)
	}
	return RolePage{List: roles, Total: total, Page: page, Size: size}, nil
}
func (service *Service) GetRole(ctx context.Context, roleID uint) (domain.Role, error) {
	role, err := service.repository.FindRole(ctx, roleID)
	if err != nil {
		return domain.Role{}, roleError(err)
	}
	return role, nil
}

func (service *Service) UserRoleSummaries(ctx context.Context, userIDs []uint) ([]UserRoleSummary, error) {
	summaries, err := service.repository.UserRoleSummaries(ctx, userIDs)
	if err != nil {
		return nil, NewError(CodeInternalError, err)
	}
	if summaries == nil {
		return []UserRoleSummary{}, nil
	}
	return summaries, nil
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
	var role domain.Role
	err := service.transactions.Run(ctx, func(tx context.Context) error {
		operation := func(tx context.Context) error {
			var err error
			role, err = service.repository.LockRole(tx, roleID)
			if err != nil {
				return roleError(err)
			}
			if domain.IsProtectedRole(role.Code) {
				return NewError(CodeConflict, nil)
			}
			if request.Name == nil && request.Code == nil && request.Description == nil && request.Sort == nil && request.Status == nil && request.DataScope == "" {
				return NewError(CodeValidationInvalid, nil)
			}
			current := role
			if request.Name != nil {
				if strings.TrimSpace(*request.Name) == "" {
					return NewError(CodeValidationInvalid, nil)
				}
				exists, err := service.repository.RoleNameExists(tx, *request.Name, roleID)
				if err != nil {
					return wrapError(err)
				}
				if exists {
					return NewError(CodeConflict, nil)
				}
				role.Name = *request.Name
			}
			if request.Code != nil {
				if strings.TrimSpace(*request.Code) == "" {
					return NewError(CodeValidationInvalid, nil)
				}
				exists, err := service.repository.RoleCodeExists(tx, *request.Code, roleID)
				if err != nil {
					return wrapError(err)
				}
				if exists {
					return NewError(CodeConflict, nil)
				}
				role.Code = *request.Code
			}
			if request.Description != nil {
				role.Description = *request.Description
			}
			if request.Sort != nil {
				role.Sort = *request.Sort
			}
			if request.Status != nil {
				role.Status = *request.Status
			}
			if request.DataScope != "" {
				role.DataScope, err = domain.ParseDataScope(request.DataScope)
				if err != nil || role.DataScope == domain.DataScopeCustom {
					return NewError(CodeValidationInvalid, err)
				}
			}
			scopeChanged := false
			if request.DataScope != "" {
				organizationIDs, err := service.repository.CurrentCustomOrganizationIDs(tx, roleID)
				if err != nil {
					return wrapError(err)
				}
				scopeChanged = role.DataScope != current.DataScope || len(organizationIDs) != 0
			}
			if role == current && !scopeChanged {
				return nil
			}
			if role != current {
				if err := service.repository.UpdateRole(tx, role); err != nil {
					return wrapError(err)
				}
			}
			if scopeChanged {
				if err := service.repository.ReplaceRoleDataScope(tx, roleID, role.DataScope, nil); err != nil {
					return wrapError(err)
				}
			}
			return service.incrementRoleUsers(tx, roleID)
		}
		if request.DataScope != "" {
			return service.organizations.WithLockedHierarchy(tx, operation)
		}
		return operation(tx)
	})
	if err != nil {
		if _, ok := CodeOf(err); ok {
			return domain.Role{}, err
		}
		return domain.Role{}, wrapError(err)
	}
	return role, nil
}

func (service *Service) DeleteRole(ctx context.Context, roleID uint) error {
	return service.transactions.Run(ctx, func(tx context.Context) error {
		role, err := service.repository.LockRole(tx, roleID)
		if err != nil {
			return roleError(err)
		}
		if domain.IsProtectedRole(role.Code) {
			return NewError(CodeConflict, nil)
		}
		userIDs, err := service.repository.UserIDsByRole(tx, roleID)
		if err != nil {
			return err
		}
		if err := service.roleMenus.DeleteRoleMenus(tx, roleID); err != nil {
			return err
		}
		if err := service.repository.DeleteRole(tx, roleID); err != nil {
			return err
		}
		return service.incrementUsers(tx, userIDs)
	})
}

func (service *Service) AssignUsersToRole(ctx context.Context, operatorID, roleID uint, userIDs []uint) error {
	if operatorID == 0 {
		return NewError(CodeInvalidUser, nil)
	}
	for _, userID := range userIDs {
		if userID == 0 {
			return NewError(CodeInvalidUser, nil)
		}
	}
	targetIDs := uniqueIDs(userIDs)
	return service.transactions.Run(ctx, func(tx context.Context) error {
		oldIDs, err := service.repository.UserIDsByRole(tx, roleID)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		for _, userID := range oldIDs {
			if userID == 0 {
				return NewError(CodeInvalidUser, nil)
			}
		}
		// A full replacement must also validate every user it will remove.
		checkedIDs := uniqueIDs(append(oldIDs, targetIDs...))
		identityIDs := uniqueIDs(append(checkedIDs, operatorID))
		users, err := service.managedUsers.ManagementUsersByIDs(tx, identityIDs)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		existing := make(map[uint]ManagedUserFact, len(users))
		for _, user := range users {
			existing[user.ID] = user
		}
		for _, userID := range checkedIDs {
			user, exists := existing[userID]
			// Only actually removed old members may use deleted identity facts.
			if !exists || (user.Deleted && containsID(targetIDs, userID)) {
				return NewError(CodeInvalidUser, nil)
			}
		}
		roles, protected, _, err := service.lockUserRoleTargets(tx, identityIDs, []uint{roleID}, nil)
		if err != nil {
			if err == ErrNotFound {
				return roleError(err)
			}
			return err
		}
		if domain.IsProtectedRole(roles[roleID].Code) {
			return NewError(CodeConflict, nil)
		}
		currentIDs, err := service.repository.UserIDsByRole(tx, roleID)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		if !sameIDs(currentIDs, oldIDs) {
			return NewError(CodeConflict, nil)
		}
		scope, err := service.ResolveUserManagementScope(tx, domain.Principal{UserID: operatorID})
		if err != nil {
			return err
		}
		for _, userID := range checkedIDs {
			if userID == 0 || userID == operatorID || (!scope.All && !containsID(scope.UserIDs, userID)) {
				return NewError(CodeInvalidUser, nil)
			}
		}
		for _, userID := range checkedIDs {
			if containsID(protected, userID) {
				return NewError(CodeConflict, nil)
			}
		}
		added, removed := membershipChanges(currentIDs, targetIDs)
		if len(added) == 0 && len(removed) == 0 {
			return nil
		}
		if err := service.repository.ApplyRoleUserChanges(tx, roleID, added, removed); err != nil {
			return NewError(CodeInternalError, err)
		}
		return service.incrementUsers(tx, append(added, removed...))
	})
}
func (service *Service) SetUserRoles(ctx context.Context, operatorID, targetUserID uint, request UpdateUserRolesRequest) (int, error) {
	if operatorID == 0 || targetUserID == 0 || operatorID == targetUserID {
		return 0, NewError(CodeInvalidUser, nil)
	}
	if request.ExpectedAccessVersion < 1 {
		return 0, NewError(CodeValidationInvalid, nil)
	}
	for _, roleID := range request.RoleIDs {
		if roleID == 0 {
			return 0, NewError(CodeValidationInvalid, nil)
		}
	}
	roleIDs := uniqueIDs(request.RoleIDs)

	version := 0
	err := service.transactions.Run(ctx, func(tx context.Context) error {
		version = 0
		current, err := service.versions.Ensure(tx, targetUserID)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		if current != request.ExpectedAccessVersion {
			return NewError(CodeConflict, nil)
		}
		users, err := service.managedUsers.ManagementUsersByIDs(tx, uniqueIDs([]uint{operatorID, targetUserID}))
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		found := false
		for _, user := range users {
			if user.ID == targetUserID && !user.Deleted {
				found = true
			}
		}
		if !found {
			return NewError(CodeInvalidUser, nil)
		}
		roles, protected, currentRoles, err := service.lockUserRoleTargets(tx, []uint{operatorID, targetUserID}, roleIDs, nil)
		if err != nil {
			if err == ErrNotFound {
				return NewError(CodeValidationInvalid, err)
			}
			return err
		}
		for _, roleID := range roleIDs {
			if domain.IsProtectedRole(roles[roleID].Code) {
				return NewError(CodeConflict, nil)
			}
		}
		scope, err := service.ResolveUserManagementScope(tx, domain.Principal{UserID: operatorID})
		if err != nil {
			return err
		}
		if !scope.All && !containsID(scope.UserIDs, targetUserID) {
			return NewError(CodeInvalidUser, nil)
		}
		if containsID(protected, targetUserID) {
			return NewError(CodeConflict, nil)
		}
		added, removed := membershipChanges(currentRoles[targetUserID], roleIDs)
		if len(added) == 0 && len(removed) == 0 {
			version = current
			return nil
		}
		if err := service.repository.ApplyUserRoleChanges(tx, targetUserID, added, removed); err != nil {
			return NewError(CodeInternalError, err)
		}
		version, err = service.versions.EnsureAndIncrement(tx, targetUserID)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		return nil
	})
	if err != nil {
		if _, ok := CodeOf(err); ok {
			return 0, err
		}
		return 0, NewError(CodeInternalError, err)
	}
	return version, nil
}

// lockUserRoleTargets gathers before locking, then rejects changed dependencies
// rather than acquiring a newly discovered lower-ID role out of order.
func (service *Service) lockUserRoleTargets(ctx context.Context, userIDs, targetIDs, inspectionIDs []uint) (map[uint]domain.Role, []uint, map[uint][]uint, error) {
	userIDs = uniqueIDs(userIDs)
	references := make(map[uint][]uint, len(userIDs))
	roleIDs := append(append([]uint{}, targetIDs...), inspectionIDs...)
	for _, userID := range userIDs {
		ids, err := service.repository.RoleIDsByUser(ctx, userID)
		if err != nil {
			return nil, nil, nil, NewError(CodeInternalError, err)
		}
		references[userID] = ids
		roleIDs = append(roleIDs, ids...)
	}
	referencedRoles := make(map[uint]bool)
	for _, ids := range references {
		for _, roleID := range ids {
			referencedRoles[roleID] = true
		}
	}
	roles := make(map[uint]domain.Role, len(roleIDs))
	for _, roleID := range uniqueIDs(roleIDs) {
		role, err := service.repository.LockRole(ctx, roleID)
		if err != nil {
			if errors.Is(err, ErrNotFound) && containsID(targetIDs, roleID) {
				return nil, nil, nil, ErrNotFound
			}
			// Historical references to absent/deleted roles do not become live
			// custom scopes. Missing current user-role dependencies still conflict.
			if errors.Is(err, ErrNotFound) && !referencedRoles[roleID] && containsID(inspectionIDs, roleID) {
				continue
			}
			if errors.Is(err, ErrNotFound) {
				return nil, nil, nil, NewError(CodeConflict, err)
			}
			return nil, nil, nil, NewError(CodeInternalError, err)
		}
		roles[roleID] = role
	}
	var protected []uint
	for _, userID := range userIDs {
		current, err := service.repository.RoleIDsByUser(ctx, userID)
		if err != nil {
			return nil, nil, nil, NewError(CodeInternalError, err)
		}
		if !sameIDs(current, references[userID]) {
			return nil, nil, nil, NewError(CodeConflict, nil)
		}
		for _, roleID := range current {
			if domain.IsProtectedRole(roles[roleID].Code) {
				protected = append(protected, userID)
				break
			}
		}
	}
	return roles, protected, references, nil
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
	if scope != domain.DataScopeCustom && len(organizationIDs) != 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	for _, organizationID := range organizationIDs {
		if organizationID == 0 {
			return NewError(CodeValidationInvalid, nil)
		}
	}
	organizationIDs = uniqueIDs(organizationIDs)
	if scope == domain.DataScopeCustom && len(organizationIDs) == 0 {
		return NewError(CodeValidationInvalid, nil)
	}
	return service.transactions.Run(ctx, func(tx context.Context) error {
		return service.organizations.WithLockedHierarchy(tx, func(tx context.Context) error {
			role, err := service.repository.LockRole(tx, roleID)
			if err != nil {
				return roleError(err)
			}
			if domain.IsProtectedRole(role.Code) {
				return NewError(CodeConflict, nil)
			}
			if scope == domain.DataScopeCustom {
				existing, err := service.organizations.ExistingOrganizationIDs(tx, organizationIDs)
				if err != nil {
					return NewError(CodeInternalError, err)
				}
				if !sameIDs(existing, organizationIDs) {
					return NewError(CodeValidationInvalid, nil)
				}
			}
			currentIDs, err := service.repository.CurrentCustomOrganizationIDs(tx, roleID)
			if err != nil {
				return err
			}
			if role.DataScope == scope && slices.Equal(currentIDs, organizationIDs) {
				return nil
			}
			if err := service.repository.ReplaceRoleDataScope(tx, roleID, scope, organizationIDs); err != nil {
				return err
			}
			return service.incrementRoleUsers(tx, roleID)
		})
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
	Name  *string
	Group *string
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
	var permission domain.Permission
	err := service.transactions.Run(ctx, func(tx context.Context) error {
		var err error
		permission, err = service.repository.LockPermission(tx, permissionID)
		if err != nil {
			return permissionError(err)
		}
		if request.Name == nil && request.Group == nil && request.Sort == nil {
			return NewError(CodeValidationInvalid, nil)
		}
		if request.Name != nil {
			if strings.TrimSpace(*request.Name) == "" {
				return NewError(CodeValidationInvalid, nil)
			}
			permission.Name = *request.Name
		}
		if request.Group != nil {
			permission.Group = *request.Group
		}
		if request.Sort != nil {
			permission.Sort = *request.Sort
		}
		return service.repository.UpdatePermission(tx, permission)
	})
	if err != nil {
		if _, ok := CodeOf(err); ok {
			return domain.Permission{}, err
		}
		return domain.Permission{}, wrapError(err)
	}
	return permission, nil
}

func (service *Service) DeletePermission(ctx context.Context, permissionID uint) error {
	return service.transactions.Run(ctx, func(tx context.Context) error {
		if _, err := service.repository.LockPermission(tx, permissionID); err != nil {
			return permissionError(err)
		}
		roleIDs, err := service.repository.RoleIDsByPermission(tx, permissionID)
		if err != nil {
			return err
		}
		for _, roleID := range uniqueIDs(roleIDs) {
			if _, err := service.repository.LockRole(tx, roleID); err != nil {
				return roleError(err)
			}
		}
		if err := service.repository.DeletePermission(tx, permissionID); err != nil {
			return err
		}
		return service.incrementRoles(tx, roleIDs)
	})
}

func (service *Service) AssignPermissionsToRole(ctx context.Context, roleID uint, permissionIDs []uint) error {
	return service.transactions.Run(ctx, func(tx context.Context) error {
		role, err := service.repository.LockRole(tx, roleID)
		if err != nil {
			return roleError(err)
		}
		if domain.IsProtectedRole(role.Code) {
			return NewError(CodeConflict, nil)
		}
		for _, permissionID := range permissionIDs {
			if permissionID == 0 {
				return NewError(CodeValidationInvalid, nil)
			}
		}
		targetIDs := uniqueIDs(permissionIDs)
		for _, permissionID := range targetIDs {
			if _, err := service.repository.LockPermission(tx, permissionID); err != nil {
				if errors.Is(err, ErrNotFound) {
					return NewError(CodeValidationInvalid, err)
				}
				return NewError(CodeInternalError, err)
			}
		}
		currentIDs, err := service.repository.CurrentRolePermissionIDs(tx, roleID)
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		if slices.Equal(currentIDs, targetIDs) {
			return nil
		}
		if err := service.repository.ReplaceRolePermissions(tx, roleID, targetIDs); err != nil {
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
	return service.resolveUserScope(ctx, principal, false)
}

// ResolveUserManagementScope uses current locked facts only for a management
// mutation. Ordinary snapshot readers retain their repeatable-read semantics.
// The caller already holds its gathered user/role targets in the same transaction.
func (service *Service) ResolveUserManagementScope(ctx context.Context, principal domain.Principal) (domain.UserScope, error) {
	var scope domain.UserScope
	err := service.organizations.WithLockedHierarchy(ctx, func(tx context.Context) error {
		var err error
		scope, err = service.resolveUserScope(tx, principal, true)
		return err
	})
	return scope, err
}

func (service *Service) resolveUserScope(ctx context.Context, principal domain.Principal, current bool) (domain.UserScope, error) {
	organizationScope, err := service.resolveOrganizationScope(ctx, principal, current)
	if err != nil {
		return domain.UserScope{}, wrapError(err)
	}
	if organizationScope.All {
		return domain.AllUsersScope(), nil
	}
	userIDs := []uint{principal.UserID}
	for _, organizationID := range organizationScope.OrganizationIDs {
		var ids []uint
		var err error
		if current {
			ids, err = service.organizations.CurrentMemberUserIDs(ctx, organizationID)
		} else {
			ids, err = service.organizations.MemberUserIDs(ctx, organizationID)
		}
		if err != nil {
			return domain.UserScope{}, NewError(CodeInternalError, err)
		}
		userIDs = append(userIDs, ids...)
	}
	return domain.UserScopeForIDs(userIDs), nil
}

func (service *Service) ResolveOrganizationScope(ctx context.Context, principal domain.Principal) (domain.OrganizationScope, error) {
	return service.resolveOrganizationScope(ctx, principal, false)
}

// ResolveOrganizationManagementScope is used after the caller's hierarchy and
// current management dependencies have been locked, never by runtime snapshots.
func (service *Service) ResolveOrganizationManagementScope(ctx context.Context, principal domain.Principal) (domain.OrganizationScope, error) {
	return service.resolveOrganizationScope(ctx, principal, true)
}

func (service *Service) resolveOrganizationScope(ctx context.Context, principal domain.Principal, current bool) (domain.OrganizationScope, error) {
	if principal.UserID == 0 {
		return domain.OrganizationScope{}, ErrInvalidUser
	}
	all, ids, err := service.scopeOrganizations(ctx, principal.UserID, current)
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

func (service *Service) scopeOrganizations(ctx context.Context, operatorID uint, current bool) (bool, []uint, error) {
	var roles []domain.Role
	var err error
	if current {
		roles, err = service.repository.CurrentRolesForUser(ctx, operatorID)
	} else {
		roles, err = service.repository.RolesForUser(ctx, operatorID)
	}
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
		case domain.DataScopeOrg, domain.DataScopeOrgAndChildren:
			var ids []uint
			var err error
			if current {
				ids, err = service.organizations.CurrentMemberOrganizationIDs(ctx, operatorID)
			} else {
				ids, err = service.organizations.MemberOrganizationIDs(ctx, operatorID)
			}
			if err != nil {
				return false, nil, wrapError(err)
			}
			if role.DataScope == domain.DataScopeOrgAndChildren {
				ids, err = service.organizations.DescendantOrganizationIDs(ctx, ids)
				if err != nil {
					return false, nil, wrapError(err)
				}
			}
			organizationIDs = append(organizationIDs, ids...)
		case domain.DataScopeCustom:
			var ids []uint
			var err error
			if current {
				ids, err = service.repository.CurrentCustomOrganizationIDs(ctx, role.ID)
			} else {
				ids, err = service.repository.CustomOrganizationIDs(ctx, role.ID)
			}
			if err != nil {
				return false, nil, wrapError(err)
			}
			organizationIDs = append(organizationIDs, ids...)
		}
	}
	return false, uniqueIDs(organizationIDs), nil
}

// HierarchyDependentUserIDs reads individual role dependencies, not merged scope:
// an all/admin role must not hide another role's org_and_children dependency.
// The caller holds Organization's hierarchy barrier and keeps this context's
// transaction open through the hierarchy write and access-version increments.
func (service *Service) HierarchyDependentUserIDs(ctx context.Context, changedRootIDs []uint) ([]uint, error) {
	roots := uniqueIDs(changedRootIDs)
	if len(roots) == 0 {
		return []uint{}, nil
	}
	userIDs, err := service.organizations.CurrentMemberUserIDsByOrganizations(ctx, roots)
	if err != nil {
		return nil, wrapError(err)
	}
	userIDs = uniqueIDs(userIDs)
	if len(userIDs) == 0 {
		return []uint{}, nil
	}
	references, err := service.repository.CurrentUserRoleReferences(ctx, userIDs)
	if err != nil {
		return nil, wrapError(err)
	}
	roleIDs := make([]uint, 0, len(references))
	for _, reference := range references {
		roleIDs = append(roleIDs, reference.RoleID)
	}
	roleIDs = uniqueIDs(roleIDs)
	if len(roleIDs) == 0 {
		return []uint{}, nil
	}
	// Do not filter disabled/scope metadata before locking: an enable or scope
	// writer may have won while this operation waited for the role barrier.
	roles, err := service.repository.LockRoles(ctx, roleIDs)
	if err != nil {
		return nil, wrapError(err)
	}
	eligible := make(map[uint]bool, len(roles))
	for _, role := range roles {
		eligible[role.ID] = role.Status == 1 && role.DataScope == domain.DataScopeOrgAndChildren && !domain.IsProtectedRole(role.Code)
	}
	current, err := service.repository.CurrentUserRoleReferences(ctx, userIDs)
	if err != nil {
		return nil, wrapError(err)
	}
	// Deleted-role winners (and historical orphan references) cannot contribute
	// a dependency. Other reference changes conflict rather than acquiring newly
	// discovered role locks outside the established ascending order.
	liveReferences := references[:0]
	for _, reference := range references {
		if _, exists := eligible[reference.RoleID]; exists {
			liveReferences = append(liveReferences, reference)
		}
	}
	liveCurrent := current[:0]
	for _, reference := range current {
		if _, gathered := slices.BinarySearch(roleIDs, reference.RoleID); !gathered {
			return nil, NewError(CodeConflict, nil)
		}
		if _, exists := eligible[reference.RoleID]; exists {
			liveCurrent = append(liveCurrent, reference)
		}
	}
	if !slices.Equal(liveReferences, liveCurrent) {
		return nil, NewError(CodeConflict, nil)
	}
	dependents := []uint{}
	for _, reference := range liveCurrent {
		if eligible[reference.RoleID] && (len(dependents) == 0 || dependents[len(dependents)-1] != reference.UserID) {
			dependents = append(dependents, reference.UserID)
		}
	}
	return dependents, nil
}

// ProtectedUserIDs locks only actual user-role dependencies, not an unmatched
// administrator row from a joined policy query. Callers include the operator
// before reading its scope so all role targets are gathered in one ID order.
func (service *Service) ProtectedUserIDs(ctx context.Context, userIDs []uint) ([]uint, error) {
	userIDs = uniqueIDs(userIDs)
	if len(userIDs) == 0 {
		return []uint{}, nil
	}
	if _, err := service.managedUsers.ManagementUsersByIDs(ctx, userIDs); err != nil {
		return nil, wrapError(err)
	}
	_, protected, _, err := service.lockUserRoleTargets(ctx, userIDs, nil, nil)
	return protected, err
}

func (service *Service) HasCustomOrganizationReferences(ctx context.Context, organizationID uint) (bool, error) {
	return service.lockOrganizationReferences(ctx, organizationID, nil)
}

// LockOrganizationReferences gathers the operator, current members and raw
// custom-reference roles before any policy scope read acquires role locks.
// The caller already holds Organization's hierarchy barrier.
func (service *Service) LockOrganizationReferences(ctx context.Context, operatorID, organizationID uint, memberIDs []uint) (bool, error) {
	userIDs := uniqueIDs(append(append([]uint{}, memberIDs...), operatorID))
	if _, err := service.managedUsers.ManagementUsersByIDs(ctx, userIDs); err != nil {
		return false, wrapError(err)
	}
	return service.lockOrganizationReferences(ctx, organizationID, userIDs)
}

func (service *Service) lockOrganizationReferences(ctx context.Context, organizationID uint, userIDs []uint) (bool, error) {
	references, err := service.repository.OrganizationReferenceRoleIDs(ctx, organizationID)
	if err != nil {
		return false, wrapError(err)
	}
	roles, _, _, err := service.lockUserRoleTargets(ctx, userIDs, nil, references)
	if err != nil {
		return false, err
	}
	current, err := service.repository.OrganizationReferenceRoleIDs(ctx, organizationID)
	if err != nil {
		return false, wrapError(err)
	}
	if !sameIDs(references, current) {
		return false, NewError(CodeConflict, nil)
	}
	for _, roleID := range current {
		role, exists := roles[roleID]
		if exists && !domain.IsProtectedRole(role.Code) && role.DataScope == domain.DataScopeCustom {
			return true, nil
		}
	}
	return false, nil
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
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
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

// membershipChanges compares the sorted, unique IDs returned by relationship
// reads with a normalized target. Retained relationships are never rewritten.
func membershipChanges(current, target []uint) (added, removed []uint) {
	left, right := 0, 0
	for left < len(current) && right < len(target) {
		switch {
		case current[left] < target[right]:
			removed = append(removed, current[left])
			left++
		case target[right] < current[left]:
			added = append(added, target[right])
			right++
		default:
			left++
			right++
		}
	}
	added = append(added, target[right:]...)
	removed = append(removed, current[left:]...)
	return added, removed
}

func containsID(ids []uint, target uint) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
