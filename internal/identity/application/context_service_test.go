package application_test

import (
	"context"
	"testing"

	"admin/internal/identity/application"
	"admin/internal/identity/domain"
)

type contextAuthorizationFake struct{}

func (contextAuthorizationFake) EnsureVersion(context.Context, uint) (int, error) { return 3, nil }
func (contextAuthorizationFake) Snapshot(context.Context, uint) (domain.AccessSnapshot, error) {
	return domain.AccessSnapshot{Roles: []string{"operator"}, Permissions: []string{"users.read"}, Version: 3}, nil
}

type contextNavigationFake struct{}

func (contextNavigationFake) UserMenus(context.Context, uint) ([]domain.Menu, error) {
	return []domain.Menu{{ID: 5, Name: "Users", Children: []domain.Menu{}}}, nil
}

func TestContextServiceCombinesIdentityAuthorizationAndNavigation(t *testing.T) {
	users := &userRepositoryFake{user: domain.User{ID: 7, Username: "alice", Status: 1}}
	service := application.NewContextService(users, contextAuthorizationFake{}, contextNavigationFake{})
	result, err := service.Load(context.Background(), 7)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if result.User.ID != 7 || len(result.Roles) != 1 || result.Roles[0] != "operator" || len(result.Permissions) != 1 || result.Permissions[0] != "users.read" || len(result.Menus) != 1 || result.Menus[0].ID != 5 {
		t.Fatalf("context result = %#v", result)
	}
}
