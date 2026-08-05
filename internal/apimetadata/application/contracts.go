package application

import (
	"context"

	"admin/internal/apimetadata/domain"
)

type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}

type PermissionCoordinator interface {
	ChangeAPIPermissionCode(context.Context, uint, string, func(context.Context) error) error
	DeleteAPI(context.Context, uint, func(context.Context) error) error
	EnsureAPIPermission(context.Context, domain.API) (bool, error)
	GenerateMenuButton(context.Context, uint, uint, string, int) (Button, error)
}
