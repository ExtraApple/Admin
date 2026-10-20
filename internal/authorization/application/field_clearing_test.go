package application_test

import (
	"context"
	"reflect"
	"testing"

	"admin/internal/authorization/application"
)

func TestRoleApplicationRejectsExplicitEmptyIdentifiersBeforeClearing(t *testing.T) {
	for _, field := range []string{"name", "code"} {
		t.Run(field, func(t *testing.T) {
			fixture := newAuthorizationFixture(t)
			ctx := context.Background()
			role, err := fixture.service.CreateRole(ctx, application.CreateRoleRequest{Name: "Keep name", Code: "keep-code", Description: "Keep description", Status: 1, DataScope: "self"})
			if err != nil {
				t.Fatal(err)
			}
			empty := ""
			request := application.UpdateRoleRequest{Description: &empty}
			if field == "name" {
				request.Name = &empty
			} else {
				request.Code = &empty
			}
			if _, err := fixture.service.UpdateRole(ctx, role.ID, request); !isClearingValidation(err) {
				t.Fatalf("direct explicit empty %s error=%v", field, err)
			}
			stored, err := fixture.repository.FindRole(ctx, role.ID)
			if err != nil || !reflect.DeepEqual(role, stored) {
				t.Fatalf("rejected direct update changed role: before=%#v after=%#v error=%v", role, stored, err)
			}
		})
	}
}

func TestPermissionApplicationRejectsExplicitEmptyNameBeforeClearing(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	ctx := context.Background()
	permission, err := fixture.service.CreatePermission(ctx, application.CreatePermissionRequest{Name: "Keep name", Code: "keep.permission", Group: "Keep group"})
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	_, err = fixture.service.UpdatePermission(ctx, permission.ID, application.UpdatePermissionRequest{Name: &empty, Group: &empty})
	if !isClearingValidation(err) {
		t.Fatalf("direct explicit empty permission name error=%v", err)
	}
	stored, err := fixture.repository.FindPermission(ctx, permission.ID)
	if err != nil || !reflect.DeepEqual(permission, stored) {
		t.Fatalf("rejected direct update changed permission: before=%#v after=%#v error=%v", permission, stored, err)
	}
}

func isClearingValidation(err error) bool {
	code, found := application.CodeOf(err)
	return found && code == application.CodeValidationInvalid
}
