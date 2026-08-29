package application_test

import (
	"context"
	"reflect"
	"testing"

	"admin/internal/messaging/application"
)

type authorizationReaderFake struct{}

func (authorizationReaderFake) HasPermission(context.Context, uint, string) (bool, error) {
	return true, nil
}

func (authorizationReaderFake) OrganizationScope(context.Context, uint) (application.MessageOrganizationScope, error) {
	return application.MessageOrganizationScope{OrganizationIDs: []uint{10, 11}}, nil
}

var _ application.AuthorizationReader = authorizationReaderFake{}

func TestAuthorizationContractSeparatesPermissionAndOrganizationScope(t *testing.T) {
	reader := authorizationReaderFake{}
	allowed, err := reader.HasPermission(context.Background(), 7, "admin.messages.broadcast.manage")
	if err != nil || !allowed {
		t.Fatalf("HasPermission() = %v, %v", allowed, err)
	}
	scope, err := reader.OrganizationScope(context.Background(), 7)
	if err != nil || scope.All || !reflect.DeepEqual(scope.OrganizationIDs, []uint{10, 11}) {
		t.Fatalf("OrganizationScope() = %#v, %v", scope, err)
	}
}
