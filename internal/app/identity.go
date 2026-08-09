package app

import (
	"context"
	"time"

	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	identitygorm "admin/internal/identity/adapters/gorm"
	identityhttp "admin/internal/identity/adapters/http"
	identityjwt "admin/internal/identity/adapters/jwt"
	identitypassword "admin/internal/identity/adapters/password"
	identityredis "admin/internal/identity/adapters/redis"
	identityapplication "admin/internal/identity/application"
	identitydomain "admin/internal/identity/domain"
	navigationmodule "admin/internal/navigation"
	platformconfig "admin/internal/platform/config"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
)

type identityCore struct {
	repository *identitygorm.Repository
	directory  *identityapplication.DirectoryService
}

func newIdentityCore(resources Resources) identityCore {
	repository := identitygorm.NewRepository(resources.DB)
	return identityCore{repository: repository, directory: identityapplication.NewDirectoryService(repository)}
}

type identityComposition struct {
	service    *identityapplication.Service
	users      *identityapplication.UserService
	context    *identityapplication.ContextService
	avatars    *identityapplication.AvatarService
	store      *identityredis.Store
	repository identityapplication.UserRepository
}

func newIdentityComposition(resources Resources, config platformconfig.Config, authorization *authapplication.Service, navigation identityapplication.NavigationReader, core identityCore) identityComposition {
	store := identityredis.NewStore(resources.Redis)
	tokens := identityjwt.NewService(identityjwt.Config{
		Secret:                  config.Jwt.Secret,
		AccessExpireMins:        config.Jwt.Expire,
		RefreshExpireMins:       config.Jwt.RefreshExpire,
		LegacyAccessExpireMins:  config.Jwt.LegacyAccessExpire,
		LegacyRefreshExpireMins: config.Jwt.LegacyRefreshExpire,
	})
	authorizationReader := identityAuthorization{authorization: authorization}
	service := identityapplication.NewService(
		core.repository,
		store,
		identitypassword.Bcrypt{},
		store,
		store,
		authorizationReader,
		tokens,
	)
	access := &identityAccessManager{authorization: authorization, roles: authgorm.NewRepository(resources.DB), versions: authgorm.NewAccessVersions(resources.DB)}
	users := identityapplication.NewUserService(core.repository, identitypassword.Bcrypt{}, platformdatabase.NewTransactionRunner(resources.DB), access)
	contextService := identityapplication.NewContextService(core.repository, authorizationReader, navigation)
	avatars := newAvatarService(resources, core.repository)
	return identityComposition{service: service, users: users, context: contextService, avatars: avatars, store: store, repository: core.repository}
}

func identityDescriptors(composition identityComposition, config platformconfig.Config) []routecatalog.Descriptor {
	return identityhttp.Routes(composition.service, composition.users, composition.context, composition.avatars, composition.store, config.FileUpload.AvatarMaxSizeMB, durationMinutes(config.Jwt.Expire))
}

func durationMinutes(minutes int) time.Duration { return time.Duration(minutes) * time.Minute }

type identityAuthorization struct{ authorization *authapplication.Service }

func (adapter identityAuthorization) EnsureVersion(ctx context.Context, userID uint) (int, error) {
	return adapter.authorization.EnsureAccessVersion(ctx, userID)
}

func (adapter identityAuthorization) Snapshot(ctx context.Context, userID uint) (identitydomain.AccessSnapshot, error) {
	snapshot, err := adapter.authorization.Snapshot(ctx, authdomain.Principal{UserID: userID})
	if err != nil {
		return identitydomain.AccessSnapshot{}, err
	}
	return identitydomain.AccessSnapshot{Roles: append([]string(nil), snapshot.Roles...), Permissions: append([]string(nil), snapshot.Permissions...), Version: snapshot.Version}, nil
}

var _ identityapplication.AuthorizationReader = identityAuthorization{}

type identityNavigation struct{ navigation *navigationmodule.Service }

func (adapter identityNavigation) UserMenus(ctx context.Context, userID uint) ([]identitydomain.Menu, error) {
	menus, err := adapter.navigation.UserMenus(ctx, userID)
	if err != nil {
		return nil, err
	}
	return identityMenus(menus), nil
}

func identityMenus(menus []navigationmodule.MenuDetail) []identitydomain.Menu {
	result := make([]identitydomain.Menu, len(menus))
	for index, menu := range menus {
		result[index] = identitydomain.Menu{ID: menu.ID, ParentID: menu.ParentID, Name: menu.Name, Path: menu.Path, Component: menu.Component, Icon: menu.Icon, PermissionCode: menu.PermissionCode, Sort: menu.Sort, Type: menu.Type, Status: menu.Status, Children: identityMenus(menu.Children)}
	}
	return result
}

var _ identityapplication.NavigationReader = identityNavigation{}

type identityAccessManager struct {
	authorization *authapplication.Service
	roles         *authgorm.Repository
	versions      *authgorm.AccessVersions
}

func (adapter *identityAccessManager) UserScope(ctx context.Context, userID uint) (identitydomain.UserScope, error) {
	scope, err := adapter.authorization.ResolveUserScope(ctx, authdomain.Principal{UserID: userID})
	if err != nil {
		return identitydomain.UserScope{}, err
	}
	return identitydomain.UserScope{All: scope.All, UserIDs: append([]uint(nil), scope.UserIDs...)}, nil
}

func (adapter *identityAccessManager) IncrementVersions(ctx context.Context, userIDs []uint) error {
	return adapter.versions.Increment(ctx, userIDs)
}

func (adapter *identityAccessManager) IsAdministrator(ctx context.Context, userID uint) (bool, error) {
	roles, err := adapter.roles.RolesForUser(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, role := range roles {
		if role.Status == 1 && authdomain.IsProtectedRole(role.Code) {
			return true, nil
		}
	}
	return false, nil
}

var _ identityapplication.AccessManager = (*identityAccessManager)(nil)
