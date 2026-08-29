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

func TestAnnouncementServiceCreatesDraftScheduledAndPublishedCopies(t *testing.T) {
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

	draft, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: category.Code, Title: "Draft", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(draft) != 1 || draft[0].Status != domain.MessageStatusDraft || draft[0].PublishAt != nil {
		t.Fatalf("CreateAnnouncement draft = %#v, %v", draft, err)
	}
	scheduledAt := now.Add(time.Hour)
	scheduled, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: category.Code, Title: "Scheduled", Markdown: "Body", PublishAt: &scheduledAt, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(scheduled) != 1 || scheduled[0].Status != domain.MessageStatusScheduled || scheduled[0].PublishAt == nil || !scheduled[0].PublishAt.Equal(scheduledAt) {
		t.Fatalf("CreateAnnouncement scheduled = %#v, %v", scheduled, err)
	}
	published, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: category.Code, Title: "Published", Markdown: "Body", PublishAt: &now, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(published) != 1 || published[0].Status != domain.MessageStatusPublished || published[0].PublishAt == nil || !published[0].PublishAt.Equal(now) {
		t.Fatalf("CreateAnnouncement published = %#v, %v", published, err)
	}
	var outboxes []messaginggorm.MessageOutbox
	if err := db.Order("id asc").Find(&outboxes).Error; err != nil || len(outboxes) != 1 || outboxes[0].EventName != domain.EventNameMessagePublished {
		t.Fatalf("announcement outboxes = %#v, %v", outboxes, err)
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
	if err != nil || len(announcement) != 1 || announcement[0].Status != domain.MessageStatusPublished {
		t.Fatalf("CreateAnnouncement expiring = %#v, %v", announcement, err)
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
