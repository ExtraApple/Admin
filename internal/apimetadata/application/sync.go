package application

import (
	"context"
	"errors"
	"strings"

	"admin/internal/apimetadata/domain"
)

func (service *Service) SyncRoutes(ctx context.Context) ([]domain.API, error) {
	if service.routes == nil {
		return nil, errors.New("API 路由来源不可用")
	}
	facts, err := service.routes.Routes(ctx)
	if err != nil {
		return nil, err
	}
	if service.transactions == nil {
		return nil, errors.New("API 路由同步事务不可用")
	}
	var created []domain.API
	err = service.transactions.Run(ctx, func(transactionContext context.Context) error {
		var syncErr error
		created, syncErr = service.syncRoutesInTransaction(transactionContext, facts)
		return syncErr
	})
	return created, err
}

func (service *Service) syncRoutesInTransaction(ctx context.Context, facts []RouteFact) ([]domain.API, error) {
	created := make([]domain.API, 0)
	for _, fact := range facts {
		method := strings.ToUpper(strings.TrimSpace(fact.Method))
		path := strings.TrimSpace(fact.Path)
		if method == "" || path == "" {
			return nil, errors.New("路由 Method 和 Path 不能为空")
		}
		if !strings.HasPrefix(path, "/api/") {
			continue
		}
		api, deleted, err := service.core.repository.FindByMethodPathUnscoped(ctx, method, path)
		if err != nil {
			return nil, err
		}
		if api.ID != 0 {
			updates := make(map[string]any)
			if deleted {
				updates["deleted_at"] = nil
				updates["status"] = 1
			}
			if method == "POST" && path == "/api/refresh" {
				updates["need_auth"] = 0
				updates["permission_code"] = ""
			}
			if len(updates) > 0 {
				if err := service.core.repository.Restore(ctx, api.ID, updates); err != nil {
					return nil, errors.New("同步API公开配置失败: " + err.Error())
				}
			}
			continue
		}
		needAuth, permissionCode := routeAccessValues(fact)
		name := strings.TrimSpace(fact.Name)
		if name == "" {
			name = method + " " + path
		}
		group := strings.TrimSpace(fact.Group)
		if group == "" {
			group = InferGroup(path)
		}
		api = domain.API{Name: name, Method: method, Path: path, Group: group, PermissionCode: permissionCode, Status: 1, NeedAuth: needAuth, NeedAudit: boolInt(fact.NeedAudit)}
		if err := service.core.repository.Create(ctx, &api); err != nil {
			return nil, errors.New("同步API失败: " + err.Error())
		}
		created = append(created, api)
	}
	return created, nil
}

func routeAccessValues(fact RouteFact) (int, string) {
	needAuth := 0
	if fact.Authenticated || fact.PermissionControlled {
		needAuth = 1
	}
	permissionCode := ""
	if fact.PermissionControlled {
		permissionCode = strings.TrimSpace(fact.DefaultPermissionCode)
		if permissionCode == "" {
			permissionCode = DerivePermissionCode(fact.Method, fact.Path)
		}
	}
	return needAuth, permissionCode
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
