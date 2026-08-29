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

func TestBroadcastServiceCreatesIndependentOrganizationCopiesAndDynamicAudiences(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	for _, organizationID := range []uint{10, 11} {
		if _, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: organizationID, Code: "notice", Name: "Notice", Enabled: true}); err != nil {
			t.Fatalf("create category for organization %d: %v", organizationID, err)
		}
	}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := application.NewService(application.Dependencies{
		Messages:      repository,
		Categories:    repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10, 11}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})

	copies, err := service.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{
		ActorID:      7,
		CategoryCode: "notice",
		Title:        "System update",
		Markdown:     "Service update",
		Targets: []application.DynamicAudienceTarget{
			{OrganizationID: 10, Type: domain.AudienceTypeOrganization},
			{OrganizationID: 10, Type: domain.AudienceTypeRole, RoleID: 3},
			{OrganizationID: 11, Type: domain.AudienceTypeOrganization},
		},
	})
	if err != nil || len(copies) != 2 || copies[0].LogicalID == "" || copies[0].LogicalID != copies[1].LogicalID {
		t.Fatalf("CreateBroadcast() = %#v, %v", copies, err)
	}
	for _, copy := range copies {
		if copy.Kind != domain.MessageKindBroadcast || copy.Status != domain.MessageStatusPublished || copy.AggregateVersion != 1 || copy.CategoryID == 0 {
			t.Fatalf("broadcast copy = %#v", copy)
		}
		rules, err := repository.ListAudienceRules(context.Background(), copy.ID)
		if err != nil || len(rules) == 0 {
			t.Fatalf("ListAudienceRules(%d) = %#v, %v", copy.ID, rules, err)
		}
	}

	if _, err := service.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{ActorID: 7, CategoryCode: "notice", Title: "Out of scope", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 12, Type: domain.AudienceTypeOrganization}}}); err != application.ErrOrganizationNotManaged {
		t.Fatalf("out-of-scope CreateBroadcast() error = %v, want ErrOrganizationNotManaged", err)
	}
}

func TestBroadcastServiceCreatesAllUserCopiesOnlyForSuperAdministrator(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	for _, organizationID := range []uint{10, 11} {
		if _, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: organizationID, Code: "notice", Name: "Notice", Enabled: true}); err != nil {
			t.Fatalf("create category: %v", err)
		}
	}
	service := application.NewService(application.Dependencies{
		Messages:      repository,
		Categories:    repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{All: true}},
		Organizations: adminOrganizationFake{all: []uint{10, 11}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
	})
	copies, err := service.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{ActorID: 1, CategoryCode: "notice", Title: "All users", Markdown: "Body", AllUsers: true})
	if err != nil || len(copies) != 2 {
		t.Fatalf("super CreateBroadcast() = %#v, %v", copies, err)
	}
	for _, copy := range copies {
		rules, err := repository.ListAudienceRules(context.Background(), copy.ID)
		if err != nil || len(rules) != 1 || rules[0].Type != domain.AudienceTypeAll {
			t.Fatalf("all-user rules = %#v, %v", rules, err)
		}
	}

	limited := application.NewService(application.Dependencies{
		Messages:      repository,
		Categories:    repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10, 11}},
	})
	if _, err := limited.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{ActorID: 7, CategoryCode: "notice", Title: "All users", Markdown: "Body", AllUsers: true}); err != application.ErrOrganizationNotManaged {
		t.Fatalf("non-super AllUsers error = %v, want ErrOrganizationNotManaged", err)
	}
}

func TestBroadcastServiceRollsBackEveryCopyWhenCategoryIsMissing(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	if _, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	service := application.NewService(application.Dependencies{
		Messages:      repository,
		Categories:    repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{All: true}},
		Organizations: adminOrganizationFake{all: []uint{10, 11}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
	})
	if _, err := service.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{ActorID: 1, CategoryCode: "notice", Title: "All users", Markdown: "Body", AllUsers: true}); err == nil {
		t.Fatal("CreateBroadcast() accepted a target without matching category")
	}
	messages, total, err := repository.ListMessages(context.Background(), application.MessageListQuery{OrganizationIDs: []uint{10, 11}})
	if err != nil || total != 0 || len(messages) != 0 {
		t.Fatalf("messages after rollback = %#v total=%d err=%v", messages, total, err)
	}
	var outboxes int64
	if err := db.Model(&messaginggorm.MessageOutbox{}).Count(&outboxes).Error; err != nil || outboxes != 0 {
		t.Fatalf("outboxes after rollback = %d, %v", outboxes, err)
	}
}
