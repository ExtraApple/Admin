package application_test

import (
	"context"
	"testing"

	"admin/internal/apimetadata/application"
	"admin/internal/apimetadata/domain"
)

func TestServiceSynchronizesPermissionsFromAPIMetadata(t *testing.T) {
	core := newAPIMetadataCore(t)
	ctx := context.Background()
	authAPI, err := core.Create(ctx, application.CreateInput{Name: "List Users", Method: "GET", Path: "/api/admin/users"})
	if err != nil {
		t.Fatalf("create authenticated API: %v", err)
	}
	needAuth := 0
	if _, err := core.Create(ctx, application.CreateInput{Name: "Public", Method: "GET", Path: "/api/public", NeedAuth: &needAuth}); err != nil {
		t.Fatalf("create public API: %v", err)
	}
	coordinator := &permissionCoordinatorFake{created: map[string]bool{"admin.users.get": true}}
	service := application.NewService(core, coordinator)
	created, updated, err := service.SyncPermissions(ctx)
	if err != nil {
		t.Fatalf("sync permissions: %v", err)
	}
	if updated != 1 || len(created) != 1 || created[0] != "admin.users.get" {
		t.Fatalf("sync result created=%#v updated=%d", created, updated)
	}
	stored, err := core.Get(ctx, authAPI.ID)
	if err != nil || stored.PermissionCode != "admin.users.get" {
		t.Fatalf("stored permission code = %#v, %v", stored, err)
	}
	if len(coordinator.changed) != 1 || coordinator.changed[0] != authAPI.ID || len(coordinator.ensured) != 1 {
		t.Fatalf("coordinator changes=%#v ensured=%#v", coordinator.changed, coordinator.ensured)
	}
}

type permissionCoordinatorFake struct {
	created map[string]bool
	changed []uint
	deleted []uint
	ensured []domain.API
}

func (fake *permissionCoordinatorFake) ChangeAPIPermissionCode(ctx context.Context, apiID uint, _ string, update func(context.Context) error) error {
	fake.changed = append(fake.changed, apiID)
	return update(ctx)
}
func (fake *permissionCoordinatorFake) DeleteAPI(ctx context.Context, apiID uint, deleteAPI func(context.Context) error) error {
	fake.deleted = append(fake.deleted, apiID)
	return deleteAPI(ctx)
}

func TestServiceDelegatesPermissionCodeChangesToNavigation(t *testing.T) {
	core := newAPIMetadataCore(t)
	ctx := context.Background()
	api, err := core.Create(ctx, application.CreateInput{Name: "List", Method: "GET", Path: "/api/admin/list", PermissionCode: "old.code"})
	if err != nil {
		t.Fatalf("create API: %v", err)
	}
	coordinator := &permissionCoordinatorFake{created: map[string]bool{}}
	service := application.NewService(core, coordinator)
	newCode := "new.code"
	updated, err := service.Update(ctx, api.ID, application.UpdateInput{PermissionCode: &newCode})
	if err != nil || updated.PermissionCode != newCode {
		t.Fatalf("delegated update = %#v, %v", updated, err)
	}
	if len(coordinator.changed) != 1 || coordinator.changed[0] != api.ID {
		t.Fatalf("Navigation delegation calls = %#v", coordinator.changed)
	}
}

func TestServiceDelegatesAPIDeleteToNavigation(t *testing.T) {
	core := newAPIMetadataCore(t)
	ctx := context.Background()
	api, err := core.Create(ctx, application.CreateInput{Name: "Delete", Method: "DELETE", Path: "/api/admin/users/:id"})
	if err != nil {
		t.Fatalf("create API: %v", err)
	}
	coordinator := &permissionCoordinatorFake{created: map[string]bool{}}
	service := application.NewService(core, coordinator)
	if err := service.Delete(ctx, api.ID); err != nil {
		t.Fatalf("delete API: %v", err)
	}
	if len(coordinator.deleted) != 1 || coordinator.deleted[0] != api.ID {
		t.Fatalf("Navigation delete delegation = %#v", coordinator.deleted)
	}
}
func (fake *permissionCoordinatorFake) EnsureAPIPermission(_ context.Context, api domain.API) (bool, error) {
	fake.ensured = append(fake.ensured, api)
	return fake.created[api.PermissionCode], nil
}
func (fake *permissionCoordinatorFake) GenerateMenuButton(_ context.Context, apiID, parentID uint, name string, sort int) (application.Button, error) {
	return application.Button{ID: apiID, ParentID: parentID, Name: name, Sort: sort, Type: 3, Status: 1}, nil
}
