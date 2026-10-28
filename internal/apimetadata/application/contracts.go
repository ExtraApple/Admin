package application

import (
	"context"

	"admin/internal/apimetadata/domain"
)

type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}

// PermissionReferenceLocker stabilizes permission-code cleanup while a new API
// reference is inserted. It never creates or repairs a permission resource.
type PermissionReferenceLocker interface {
	LockPermissions(context.Context, []string) error
}

type PermissionCoordinator interface {
	WithAPIPermissionSync(context.Context, []domain.API, func(context.Context) error) error
	ExistingPermissionCodes(context.Context, []string, bool) ([]string, error)
	ChangeAPIPermissionCode(context.Context, uint, string, func(context.Context) error) error
	DeleteAPI(context.Context, uint, func(context.Context) error) error
	ChangeAPIPermissionCodeWithGuard(context.Context, uint, string, func(context.Context) error, func(context.Context) error) error
	EnsureAPIPermission(context.Context, domain.API) (bool, error)
	GenerateMenuButton(context.Context, uint, uint, string, int) (Button, error)
	ValidateMenuButtonParent(context.Context, uint) error
	GenerateMenuButtonWithGuard(context.Context, uint, uint, string, int, func(context.Context) error) (Button, error)
}
