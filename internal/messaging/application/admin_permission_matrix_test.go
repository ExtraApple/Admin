package application_test

import (
	"context"
	"testing"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type permissionMatrixAuthorizationFake struct {
	requested []string
}

func (reader *permissionMatrixAuthorizationFake) HasPermission(_ context.Context, _ uint, permission string) (bool, error) {
	reader.requested = append(reader.requested, permission)
	return false, nil
}

func (reader *permissionMatrixAuthorizationFake) OrganizationScope(context.Context, uint) (application.MessageOrganizationScope, error) {
	return application.MessageOrganizationScope{}, nil
}

func TestMessagingAdministrationPermissionMatrixUsesOperationSpecificPermissions(t *testing.T) {
	store := &categoryStoreFake{categories: map[uint]domain.MessageCategory{1: {ID: 1, OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}}}
	tests := []struct {
		name       string
		permission string
		invoke     func(*application.Service) error
	}{
		{
			name:       "category management",
			permission: application.PermissionCategoryManage,
			invoke: func(service *application.Service) error {
				_, err := service.CreateCategory(context.Background(), application.CategoryCreateRequest{ActorID: 7, OrganizationID: 10, Code: "other", Name: "Other", Enabled: true})
				return err
			},
		},
		{
			name:       "broadcast management",
			permission: application.PermissionBroadcastManage,
			invoke: func(service *application.Service) error {
				_, err := service.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{ActorID: 7, CategoryCode: "notice", Title: "Title", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
				return err
			},
		},
		{
			name:       "announcement management",
			permission: application.PermissionAnnouncementManage,
			invoke: func(service *application.Service) error {
				_, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Title", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
				return err
			},
		},
		{
			name:       "managed revocation",
			permission: application.PermissionMessageRevokeAll,
			invoke: func(service *application.Service) error {
				_, err := service.RevokeManagedMessage(context.Background(), application.ManagedRevokeRequest{ActorID: 7, MessageID: 1})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorization := &permissionMatrixAuthorizationFake{}
			service := application.NewService(application.Dependencies{
				Messages:      &privateMessageStoreFake{},
				Categories:    store,
				Authorization: authorization,
				Organizations: adminOrganizationFake{descendants: []uint{10}},
			})
			if err := test.invoke(service); err != application.ErrPermissionDenied {
				t.Fatalf("operation error = %v, want ErrPermissionDenied", err)
			}
			if len(authorization.requested) != 1 || authorization.requested[0] != test.permission {
				t.Fatalf("requested permissions = %#v, want %q", authorization.requested, test.permission)
			}
		})
	}
}
