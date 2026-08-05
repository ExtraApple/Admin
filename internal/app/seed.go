package app

import (
	"context"
	"errors"
	"strings"

	apigorm "admin/internal/apimetadata/adapters/gorm"
	apiapplication "admin/internal/apimetadata/application"
	"admin/internal/app/seeddata"
	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	platformconfig "admin/internal/platform/config"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func SeedCatalog(ctx context.Context, conf platformconfig.Config, db *gorm.DB, logger *zap.Logger, catalog *routecatalog.Catalog) error {
	return seedDescriptors(ctx, conf, db, logger, catalog.Snapshot())
}

func seedDescriptors(ctx context.Context, conf platformconfig.Config, db *gorm.DB, logger *zap.Logger, descriptors []routecatalog.Descriptor) error {
	summary := seeddata.Summary{}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		foundation, err := seeddata.Foundation(tx)
		if err != nil {
			return err
		}
		summary = foundation
		return nil
	}); err != nil {
		return err
	}
	if err := syncCatalogAPIs(ctx, db, descriptors); err != nil {
		return err
	}
	if err := syncCatalogPermissions(ctx, db, descriptors); err != nil {
		return err
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		finalized, err := seeddata.Finalize(tx, &conf)
		if err != nil {
			return err
		}
		summary.Menus = finalized.Menus
		summary.RolePermissions = finalized.RolePermissions
		summary.RoleMenus = finalized.RoleMenus
		return nil
	}); err != nil {
		return err
	}
	logger.Info("seeds completed",
		zap.Int("roles", summary.Roles), zap.Int("permission_groups", summary.PermissionGroups),
		zap.Int("apis", len(descriptors)), zap.Int("menus", summary.Menus),
		zap.Int("role_permissions", summary.RolePermissions), zap.Int("role_menus", summary.RoleMenus),
		zap.Int("dict_types", summary.DictTypes), zap.Int("dict_items", summary.DictItems),
	)
	return nil
}

func syncCatalogAPIs(ctx context.Context, db *gorm.DB, descriptors []routecatalog.Descriptor) error {
	for _, descriptor := range descriptors {
		method, err := apiapplication.NormalizeMethod(descriptor.Method)
		if err != nil {
			return err
		}
		path, err := apiapplication.NormalizePath(descriptor.Path)
		if err != nil || path == "/ping" || !strings.HasPrefix(path, "/api/") {
			continue
		}
		var existing apigorm.API
		err = db.WithContext(ctx).Unscoped().Where("method = ? AND path = ?", method, path).First(&existing).Error
		if err == nil {
			updates := map[string]any{}
			if existing.DeletedAt.Valid {
				updates["deleted_at"] = nil
				updates["status"] = 1
			}
			if method == "POST" && path == "/api/refresh" {
				updates["need_auth"] = 0
				updates["permission_code"] = ""
			}
			if len(updates) > 0 {
				if err := db.WithContext(ctx).Unscoped().Model(&existing).Updates(updates).Error; err != nil {
					return errors.New("同步API配置失败: " + err.Error())
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("查询API失败: " + err.Error())
		}
		needAuth := 1
		if descriptor.Access == routecatalog.Public {
			needAuth = 0
		}
		permissionCode := strings.TrimSpace(descriptor.DefaultPermissionCode)
		if needAuth == 1 && permissionCode == "" {
			permissionCode = apiapplication.DerivePermissionCode(method, path)
		}
		needAudit := 0
		if strings.TrimSpace(descriptor.DefaultAuditCategory) != "" {
			needAudit = 1
		}
		api := apigorm.API{
			Name: strings.TrimSpace(descriptor.Name), Method: method, Path: path,
			Group: strings.TrimSpace(descriptor.Group), PermissionCode: permissionCode,
			Status: 1, NeedAuth: needAuth, NeedAudit: needAudit,
		}
		if err := db.WithContext(ctx).Create(&api).Error; err != nil {
			return errors.New("同步API失败: " + err.Error())
		}
	}
	return nil
}

func syncCatalogPermissions(ctx context.Context, db *gorm.DB, descriptors []routecatalog.Descriptor) error {
	routes := make([]authdomain.RouteFact, 0, len(descriptors))
	for _, descriptor := range descriptors {
		method, err := apiapplication.NormalizeMethod(descriptor.Method)
		if err != nil {
			return err
		}
		path, err := apiapplication.NormalizePath(descriptor.Path)
		if err != nil || path == "/ping" || !strings.HasPrefix(path, "/api/") || descriptor.Access == routecatalog.Public {
			continue
		}
		routes = append(routes, authdomain.RouteFact{
			Method: method, Path: path, PermissionCode: descriptor.DefaultPermissionCode,
		})
	}
	authorization := authapplication.NewService(
		authgorm.NewRepository(db),
		platformdatabase.NewTransactionRunner(db),
		nil, nil, authgorm.NewAccessVersions(db),
	)
	_, err := authorization.SyncPermissions(ctx, routes)
	return err
}
