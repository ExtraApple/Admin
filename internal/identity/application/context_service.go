package application

import (
	"context"

	"admin/internal/identity/domain"
)

type ContextResult struct {
	User        domain.User
	Roles       []string
	Permissions []string
	Menus       []domain.Menu
}

type ContextService struct {
	users         UserRepository
	authorization AuthorizationReader
	navigation    NavigationReader
}

func NewContextService(users UserRepository, authorization AuthorizationReader, navigation NavigationReader) *ContextService {
	return &ContextService{users: users, authorization: authorization, navigation: navigation}
}

func (service *ContextService) Load(ctx context.Context, userID uint) (ContextResult, error) {
	user, err := service.users.FindByID(ctx, userID)
	if err != nil {
		return ContextResult{}, ErrUserNotFound
	}
	snapshot, err := service.authorization.Snapshot(ctx, userID)
	if err != nil {
		return ContextResult{}, err
	}
	menus, err := service.navigation.UserMenus(ctx, userID)
	if err != nil {
		return ContextResult{}, err
	}
	if menus == nil {
		menus = []domain.Menu{}
	}
	return ContextResult{User: user, Roles: cloneStrings(snapshot.Roles), Permissions: cloneStrings(snapshot.Permissions), Menus: menus}, nil
}
