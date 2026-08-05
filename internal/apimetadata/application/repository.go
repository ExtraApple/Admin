package application

import (
	"context"
	"errors"

	"admin/internal/apimetadata/domain"
)

var ErrNotFound = errors.New("API不存在")

type Filter struct {
	Keyword   string
	Group     string
	Method    string
	Status    *int
	NeedAuth  *int
	NeedAudit *int
}

type Repository interface {
	List(context.Context, int, int, Filter) ([]domain.API, int64, error)
	Find(context.Context, uint) (domain.API, error)
	ExistsMethodPath(context.Context, uint, string, string) (bool, error)
	ListByIDs(context.Context, []uint) ([]domain.API, error)
	ListByIDsForUpdate(context.Context, []uint) ([]domain.API, error)
	SetPermissionCode(context.Context, []uint, string) error
	CountPermissionCode(context.Context, string) (int64, error)
	FindPolicyByRoute(context.Context, string, string) (domain.Policy, error)
	FindByMethodPathUnscoped(context.Context, string, string) (domain.API, bool, error)
	Restore(context.Context, uint, map[string]any) error
	Groups(context.Context) ([]domain.GroupOption, error)
	Create(context.Context, *domain.API) error
	Update(context.Context, uint, map[string]any) error
	Delete(context.Context, uint) error
}
