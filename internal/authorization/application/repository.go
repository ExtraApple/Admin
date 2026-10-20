package application

import (
	"context"
	"errors"

	"admin/internal/authorization/domain"
)

var ErrNotFound = errors.New("authorization record not found")

type UserRoleSummary struct {
	UserID uint
	RoleID uint
	Code   string
	Name   string
	Status int
}

// UserRoleReference is an unfiltered current relationship fact. Role eligibility
// is evaluated only after the referenced role metadata has been locked.
type UserRoleReference struct {
	UserID uint
	RoleID uint
}
type Repository interface {
	ListRoles(context.Context, int, int) ([]domain.Role, int64, error)
	FindRole(context.Context, uint) (domain.Role, error)
	LockRole(context.Context, uint) (domain.Role, error)
	// LockRoles locks existing, nondeleted metadata in ascending role ID order,
	// including disabled roles. Missing/deleted candidates are omitted.
	LockRoles(context.Context, []uint) ([]domain.Role, error)
	// CurrentUserRoleReferences returns sorted, unique current references without
	// filtering role status, scope or existence.
	CurrentUserRoleReferences(context.Context, []uint) ([]UserRoleReference, error)
	OrganizationReferenceRoleIDs(context.Context, uint) ([]uint, error)
	RoleNameExists(context.Context, string, uint) (bool, error)
	RoleCodeExists(context.Context, string, uint) (bool, error)
	CreateRole(context.Context, *domain.Role) error
	UpdateRole(context.Context, domain.Role) error
	DeleteRole(context.Context, uint) error
	// Relationship ID reads return the complete sorted, unique stored set.
	UserIDsByRole(context.Context, uint) ([]uint, error)
	RoleIDsByUser(context.Context, uint) ([]uint, error)
	UserRoleSummaries(context.Context, []uint) ([]UserRoleSummary, error)
	RolePermissionFacts(context.Context, []uint) ([]RolePermissionFact, error)
	// Membership changes contain only normalized additions and removals; retained rows are untouched.
	ApplyUserRoleChanges(context.Context, uint, []uint, []uint) error
	ApplyRoleUserChanges(context.Context, uint, []uint, []uint) error
	ReplaceRoleDataScope(context.Context, uint, domain.DataScope, []uint) error
	RoleDataScope(context.Context, uint) (domain.RoleDataScope, error)
	RolesForUser(context.Context, uint) ([]domain.Role, error)
	CurrentRolesForUser(context.Context, uint) ([]domain.Role, error)
	CustomOrganizationIDs(context.Context, uint) ([]uint, error)
	CurrentCustomOrganizationIDs(context.Context, uint) ([]uint, error)

	ListPermissions(context.Context, int, int) ([]domain.Permission, int64, error)
	FindPermission(context.Context, uint) (domain.Permission, error)
	LockPermission(context.Context, uint) (domain.Permission, error)
	PermissionCodeExists(context.Context, string) (bool, error)
	CreatePermission(context.Context, *domain.Permission) error
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, uint) error
	RoleIDsByPermission(context.Context, uint) ([]uint, error)
	ReplaceRolePermissions(context.Context, uint, []uint) error
	CurrentRolePermissionIDs(context.Context, uint) ([]uint, error)
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
