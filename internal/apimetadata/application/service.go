package application

import (
	"context"
	"errors"
	"strings"

	"admin/internal/apimetadata/domain"
)

type Service struct {
	core         *Core
	coordinator  PermissionCoordinator
	transactions TransactionRunner
	routes       RouteSource
}

type ServiceOption func(*Service)

func WithRouteSource(transactions TransactionRunner, routes RouteSource) ServiceOption {
	return func(service *Service) {
		service.transactions = transactions
		service.routes = routes
	}
}

func NewService(core *Core, coordinator PermissionCoordinator, options ...ServiceOption) *Service {
	service := &Service{core: core, coordinator: coordinator}
	for _, option := range options {
		option(service)
	}
	return service
}
func (service *Service) List(ctx context.Context, page, size int, filter Filter) ([]domain.API, int64, error) {
	return service.core.List(ctx, page, size, filter)
}

func (service *Service) Get(ctx context.Context, id uint) (domain.API, error) {
	return service.core.Get(ctx, id)
}

func (service *Service) Create(ctx context.Context, input CreateInput) (domain.API, error) {
	return service.core.Create(ctx, input)
}

func (service *Service) Update(ctx context.Context, id uint, input UpdateInput) (domain.API, error) {
	if input.PermissionCode == nil || service.coordinator == nil {
		return service.core.Update(ctx, id, input)
	}
	var updated domain.API
	err := service.coordinator.ChangeAPIPermissionCode(ctx, id, strings.TrimSpace(*input.PermissionCode), func(transactionContext context.Context) error {
		var err error
		updated, err = service.core.Update(transactionContext, id, input)
		return err
	})
	return updated, err
}

func (service *Service) Delete(ctx context.Context, id uint) error {
	if service.coordinator == nil {
		return service.core.Delete(ctx, id)
	}
	return service.coordinator.DeleteAPI(ctx, id, func(transactionContext context.Context) error { return service.core.Delete(transactionContext, id) })
}

func (service *Service) Groups(ctx context.Context) ([]domain.GroupOption, error) {
	return service.core.Groups(ctx)
}

func (service *Service) Methods() []MethodOption {
	return service.core.Methods()
}

func (service *Service) GenerateMenuButton(ctx context.Context, apiID, parentID uint, name string, sort int) (Button, error) {
	if service.coordinator == nil {
		return Button{}, errors.New("Navigation 协调器不可用")
	}
	return service.coordinator.GenerateMenuButton(ctx, apiID, parentID, name, sort)
}

func (service *Service) SyncPermissions(ctx context.Context) ([]string, int, error) {
	apis, _, err := service.core.repository.List(ctx, 0, -1, Filter{})
	if err != nil {
		return nil, 0, errors.New("查询API列表失败")
	}
	created := make([]string, 0)
	updated := 0
	for _, api := range apis {
		if api.NeedAuth != 1 {
			continue
		}
		code := strings.TrimSpace(api.PermissionCode)
		if code == "" {
			code = DerivePermissionCode(api.Method, api.Path)
			if err := service.coordinator.ChangeAPIPermissionCode(ctx, api.ID, code, func(transactionContext context.Context) error {
				_, err := service.core.Update(transactionContext, api.ID, UpdateInput{PermissionCode: &code})
				return err
			}); err != nil {
				return created, updated, err
			}
			api.PermissionCode = code
			updated++
		}
		wasCreated, err := service.coordinator.EnsureAPIPermission(ctx, api)
		if err != nil {
			return created, updated, err
		}
		if wasCreated {
			created = append(created, code)
		}
	}
	return created, updated, nil
}

func DerivePermissionCode(method, path string) string {
	code := strings.TrimPrefix(path, "/api/")
	code = strings.ReplaceAll(code, ":", "")
	code = strings.ReplaceAll(code, "/", ".")
	code = strings.Trim(code, ".")
	return strings.ToLower(code + "." + strings.ToUpper(method))
}
