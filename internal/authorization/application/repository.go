package application

import (
	"context"
	"errors"

	"admin/internal/authorization/domain"
)

var ErrNotFound = errors.New("authorization record not found")

type Repository interface {
	ListRoles(context.Context, int, int) ([]domain.Role, int64, error)
	FindRole(context.Context, uint) (domain.Role, error)
	RoleNameExists(context.Context, string, uint) (bool, error)
	RoleCodeExists(context.Context, string, uint) (bool, error)
	CreateRole(context.Context, *domain.Role) error
	UpdateRole(context.Context, domain.Role) error
	DeleteRole(context.Context, uint) error
	UserIDsByRole(context.Context, uint) ([]uint, error)
	ReplaceRoleUsers(context.Context, uint, []uint) error
	ReplaceRoleDataScope(context.Context, uint, domain.DataScope, []uint) error
	RoleDataScope(context.Context, uint) (domain.RoleDataScope, error)
	RolesForUser(context.Context, uint) ([]domain.Role, error)
	CustomOrganizationIDs(context.Context, uint) ([]uint, error)

	ListPermissions(context.Context, int, int) ([]domain.Permission, int64, error)
	FindPermission(context.Context, uint) (domain.Permission, error)
	PermissionCodeExists(context.Context, string) (bool, error)
	CreatePermission(context.Context, *domain.Permission) error
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, uint) error
	RoleIDsByPermission(context.Context, uint) ([]uint, error)
	ReplaceRolePermissions(context.Context, uint, []uint) error
	PermissionsByRole(context.Context, uint) ([]domain.Permission, error)
	PermissionCodesForUser(context.Context, uint) ([]string, error)
	AllPermissionCodes(context.Context) ([]string, error)

	ListPermissionGroups(context.Context, int, int) ([]domain.PermissionGroup, int64, error)
	FindPermissionGroup(context.Context, uint) (domain.PermissionGroup, error)
	PermissionGroupNameExists(context.Context, string, uint) (bool, error)
	CreatePermissionGroup(context.Context, *domain.PermissionGroup) error
	UpdatePermissionGroup(context.Context, domain.PermissionGroup) error
	DeletePermissionGroup(context.Context, uint) error
}
