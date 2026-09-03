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

	"gorm.io/gorm"
)

func TestAnnouncementServiceCreatesDraftUntilExplicitPublish(t *testing.T) {
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
	service := newAnnouncementService(repository, db, now)

	scheduledAt := now.Add(time.Hour)
	draft, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: category.Code, Title: "Scheduled", Markdown: "Body", PublishAt: &scheduledAt, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(draft) != 1 || draft[0].Status != domain.MessageStatusDraft || draft[0].PublishAt == nil || !draft[0].PublishAt.Equal(scheduledAt) {
		t.Fatalf("CreateAnnouncement draft = %#v, %v", draft, err)
	}
	var outboxes []messaginggorm.MessageOutbox
	if err := db.Find(&outboxes).Error; err != nil || len(outboxes) != 0 {
		t.Fatalf("draft outboxes = %#v, %v", outboxes, err)
	}

	scheduled, err := service.PublishAnnouncement(context.Background(), application.PublishAnnouncementRequest{ActorID: 7, MessageID: draft[0].ID})
	if err != nil || scheduled.Status != domain.MessageStatusScheduled || scheduled.PublishAt == nil || !scheduled.PublishAt.Equal(scheduledAt) {
		t.Fatalf("PublishAnnouncement scheduled = %#v, %v", scheduled, err)
	}
	if err := db.Find(&outboxes).Error; err != nil || len(outboxes) != 0 {
		t.Fatalf("scheduled outboxes = %#v, %v", outboxes, err)
	}

	publishedCount, err := service.PublishDueAnnouncements(context.Background(), scheduledAt)
	if err != nil || publishedCount != 1 {
		t.Fatalf("PublishDueAnnouncements() = %d, %v", publishedCount, err)
	}
	if err := db.Order("id asc").Find(&outboxes).Error; err != nil || len(outboxes) != 1 || outboxes[0].EventName != domain.EventNameMessagePublished {
		t.Fatalf("published outboxes = %#v, %v", outboxes, err)
	}
}

func TestAnnouncementServiceBindsReferencedImagesInMessageTransaction(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	if _, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	files := &privateMessageFilesFake{}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := application.NewService(application.Dependencies{
		Messages: repository, Categories: repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
		Files:         files, Transactions: platformdatabase.NewTransactionRunner(db), Clock: application.ClockFunc(func() time.Time { return now }),
	})
	copies, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Notice", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}, ImageIDs: []uint{91}})
	if err != nil || len(copies) != 1 {
		t.Fatalf("CreateAnnouncement() = %#v, %v", copies, err)
	}
	if files.binding.ActorID != 7 || files.binding.MessageLogicalID != copies[0].LogicalID || len(files.binding.ImageIDs) != 1 || files.binding.ImageIDs[0] != 91 {
		t.Fatalf("image binding = %#v", files.binding)
	}
}

func TestAnnouncementServiceBindsReferencedImagesWhenPublishing(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	if _, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	files := &privateMessageFilesFake{}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := application.NewService(application.Dependencies{
		Messages: repository, Categories: repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
		Files:         files, Transactions: platformdatabase.NewTransactionRunner(db), Clock: application.ClockFunc(func() time.Time { return now }),
	})
	draft, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Notice", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(draft) != 1 {
		t.Fatalf("CreateAnnouncement() = %#v, %v", draft, err)
	}
	published, err := service.PublishAnnouncement(context.Background(), application.PublishAnnouncementRequest{ActorID: 7, MessageID: draft[0].ID, ImageIDs: []uint{93}})
	if err != nil || published.Status != domain.MessageStatusPublished {
		t.Fatalf("PublishAnnouncement() = %#v, %v", published, err)
	}
	if files.binding.ActorID != 7 || files.binding.MessageLogicalID != published.LogicalID || len(files.binding.ImageIDs) != 1 || files.binding.ImageIDs[0] != 93 {
		t.Fatalf("image binding = %#v", files.binding)
	}
}

func TestAnnouncementServicePublishesScheduledAndExpiresDueCopies(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	if _, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := newAnnouncementService(repository, db, now)
	scheduledAt := now.Add(time.Hour)
	scheduled, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Scheduled", Markdown: "Body", PublishAt: &scheduledAt, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(scheduled) != 1 {
		t.Fatalf("CreateAnnouncement() = %#v, %v", scheduled, err)
	}
	scheduledMessage, err := service.PublishAnnouncement(context.Background(), application.PublishAnnouncementRequest{ActorID: 7, MessageID: scheduled[0].ID})
	if err != nil || scheduledMessage.Status != domain.MessageStatusScheduled || scheduledMessage.PublishAt == nil || !scheduledMessage.PublishAt.Equal(scheduledAt) {
		t.Fatalf("PublishAnnouncement scheduled = %#v, %v", scheduledMessage, err)
	}
	publishedCount, err := service.PublishDueAnnouncements(context.Background(), scheduledAt)
	if err != nil || publishedCount != 1 {
		t.Fatalf("PublishDueAnnouncements() = %d, %v", publishedCount, err)
	}
	published, err := repository.FindMessage(context.Background(), scheduled[0].ID)
	if err != nil || published.Status != domain.MessageStatusPublished {
		t.Fatalf("published announcement = %#v, %v", published, err)
	}

	expiresAt := scheduledAt.Add(time.Hour)
	atScheduled := newAnnouncementService(repository, db, scheduledAt)
	announcement, err := atScheduled.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Expiring", Markdown: "Body", PublishAt: &scheduledAt, ExpiresAt: &expiresAt, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(announcement) != 1 {
		t.Fatalf("CreateAnnouncement expiring = %#v, %v", announcement, err)
	}
	if announcement[0].Status != domain.MessageStatusDraft {
		t.Fatalf("CreateAnnouncement expiring status = %q, want draft", announcement[0].Status)
	}
	announcement[0], err = atScheduled.PublishAnnouncement(context.Background(), application.PublishAnnouncementRequest{ActorID: 7, MessageID: announcement[0].ID})
	if err != nil || announcement[0].Status != domain.MessageStatusPublished {
		t.Fatalf("PublishAnnouncement expiring = %#v, %v", announcement[0], err)
	}
	expiredCount, err := service.ExpireDueAnnouncements(context.Background(), expiresAt)
	if err != nil || expiredCount != 1 {
		t.Fatalf("ExpireDueAnnouncements() = %d, %v", expiredCount, err)
	}
	expired, err := repository.FindMessage(context.Background(), announcement[0].ID)
	if err != nil || expired.Status != domain.MessageStatusExpired {
		t.Fatalf("expired announcement = %#v, %v", expired, err)
	}
}

func newAnnouncementService(repository *messaginggorm.Repository, db *gorm.DB, now time.Time) *application.Service {
	return application.NewService(application.Dependencies{
		Messages:      repository,
		Categories:    repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{All: true}},
		Organizations: adminOrganizationFake{all: []uint{10}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
}
