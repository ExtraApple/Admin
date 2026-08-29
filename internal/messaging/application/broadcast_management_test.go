package application_test

import (
	"context"
	"testing"
	"time"

	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

func TestBroadcastServiceListsScopedBroadcastsAndRejectsEdits(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	category, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := application.NewService(application.Dependencies{
		Messages:      repository,
		Categories:    repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	copies, err := service.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{ActorID: 7, CategoryCode: category.Code, Title: "Update", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(copies) != 1 {
		t.Fatalf("CreateBroadcast() = %#v, %v", copies, err)
	}

	listed, total, err := service.ListBroadcasts(context.Background(), application.ManagedMessageListRequest{ActorID: 7, Keyword: "Update", Offset: 0, Limit: 10})
	if err != nil || total != 1 || len(listed) != 1 || listed[0].ID != copies[0].ID {
		t.Fatalf("ListBroadcasts() = %#v total=%d err=%v", listed, total, err)
	}
	if _, err := service.EditBroadcast(context.Background(), application.EditBroadcastRequest{ActorID: 7, MessageID: copies[0].ID, Title: "Changed", Markdown: "Changed"}); err != application.ErrMessageImmutable {
		t.Fatalf("EditBroadcast() error = %v, want ErrMessageImmutable", err)
	}
}
