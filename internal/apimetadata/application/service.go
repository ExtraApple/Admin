package application

import (
	"context"
	"strings"

	"admin/internal/apimetadata/domain"
)

type Service struct {
	core        *Core
	coordinator PermissionCoordinator
	routes      RouteSource
}

type ServiceOption func(*Service)

func WithRouteSource(routes RouteSource) ServiceOption {
	return func(service *Service) {
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
		return Button{}, NewError(CodeInternalError, nil)
	}
	return service.coordinator.GenerateMenuButton(ctx, apiID, parentID, name, sort)
}

func (service *Service) SyncPermissions(ctx context.Context) ([]string, int, error) {
	if service.coordinator == nil || service.core.transactions == nil {
		return nil, 0, NewError(CodeInternalError, nil)
	}
	var created []string
	var updated int
	err := service.core.transactions.Run(ctx, func(transactionContext context.Context) error {
		apis, _, err := service.core.repository.List(transactionContext, 0, -1, Filter{})
		if err != nil {
			return NewError(CodeInternalError, err)
		}
		const (
			existedBefore uint8 = 1 << iota
			retainedAfter
		)
		codes := make([]string, 0, len(apis))
		codeStates := make(map[string]uint8, len(apis))
		for _, api := range apis {
			if api.NeedAuth != 1 {
				continue
			}
			code := strings.TrimSpace(api.PermissionCode)
			if code == "" {
				code = DerivePermissionCode(api.Method, api.Path)
			}
			if _, seen := codeStates[code]; !seen {
				codes = append(codes, code)
				codeStates[code] = 0
			}
		}
		attemptCreated := make([]string, 0, len(codes))
		attemptUpdated := 0
		err = service.coordinator.WithAPIPermissionSync(transactionContext, apis, func(preparedContext context.Context) error {
			before, err := service.coordinator.ExistingPermissionCodes(preparedContext, codes, true)
			if err != nil {
				return NewError(CodeInternalError, err)
			}
			for _, code := range before {
				codeStates[code] |= existedBefore
			}
			for _, api := range apis {
				if api.NeedAuth != 1 {
					continue
				}
				code := strings.TrimSpace(api.PermissionCode)
				if code == "" {
					code = DerivePermissionCode(api.Method, api.Path)
					if err := service.coordinator.ChangeAPIPermissionCode(preparedContext, api.ID, code, func(updateContext context.Context) error {
						_, err := service.core.Update(updateContext, api.ID, UpdateInput{PermissionCode: &code})
						return err
					}); err != nil {
						return err
					}
					api.PermissionCode = code
					attemptUpdated++
				}
				if _, err := service.coordinator.EnsureAPIPermission(preparedContext, api); err != nil {
					return err
				}
			}
			after, err := service.coordinator.ExistingPermissionCodes(preparedContext, codes, false)
			if err != nil {
				return NewError(CodeInternalError, err)
			}
			for _, code := range after {
				codeStates[code] |= retainedAfter
			}
			for _, code := range codes {
				if codeStates[code] == retainedAfter {
					attemptCreated = append(attemptCreated, code)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		created, updated = attemptCreated, attemptUpdated
		return nil
	})
	if err != nil {
		return nil, 0, err
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
