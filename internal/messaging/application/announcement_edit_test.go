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

func TestAnnouncementServiceEditsPublishedCopyAndPreservesReadState(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	for _, category := range []domain.MessageCategory{{OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}, {OrganizationID: 10, Code: "news", Name: "News", Enabled: true}} {
		if _, err := repository.CreateCategory(context.Background(), category); err != nil {
			t.Fatalf("create category: %v", err)
		}
	}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := newAnnouncementService(repository, db, now)
	created, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Original", Markdown: "Original body", PublishAt: &now, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(created) != 1 {
		t.Fatalf("CreateAnnouncement() = %#v, %v", created, err)
	}
	if _, err := service.PublishAnnouncement(context.Background(), application.PublishAnnouncementRequest{ActorID: 7, MessageID: created[0].ID}); err != nil {
		t.Fatalf("PublishAnnouncement() = %v", err)
	}
	identity := application.InboxIdentity{UserID: 8, MessageID: created[0].ID, Memberships: []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}, Now: now}
	if _, _, err := repository.ListInbox(context.Background(), application.InboxQuery{UserID: identity.UserID, Memberships: identity.Memberships, Now: now}); err != nil {
		t.Fatalf("initialize inbox state: %v", err)
	}
	if changed, err := repository.MarkInboxRead(context.Background(), identity, now.Add(time.Minute)); err != nil || !changed {
		t.Fatalf("MarkInboxRead() = %t, %v", changed, err)
	}

	expiresAt := now.Add(24 * time.Hour)
	edited, err := service.EditAnnouncement(context.Background(), application.EditAnnouncementRequest{ActorID: 7, MessageID: created[0].ID, CategoryCode: "news", Title: "Edited", Markdown: "Edited body", ExpiresAt: &expiresAt, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeRole, RoleID: 3}}})
	if err != nil || edited.Status != domain.MessageStatusPublished || edited.AggregateVersion != 3 || edited.Title != "Edited" || edited.CategoryID == created[0].CategoryID {
		t.Fatalf("EditAnnouncement() = %#v, %v", edited, err)
	}
	inbox, total, err := repository.ListInbox(context.Background(), application.InboxQuery{UserID: 8, Memberships: identity.Memberships, RoleIDs: []uint{3}, Now: now.Add(time.Minute)})
	if err != nil || total != 1 || len(inbox) != 1 || inbox[0].Message.ID != edited.ID || inbox[0].ReadAt == nil {
		t.Fatalf("edited inbox = %#v total=%d err=%v", inbox, total, err)
	}
	var outbox messaginggorm.MessageOutbox
	if err := db.Last(&outbox).Error; err != nil || outbox.EventName != domain.EventNameMessageEdited || outbox.AggregateVersion != 3 {
		t.Fatalf("edit outbox = %#v, %v", outbox, err)
	}
}

func TestAnnouncementServiceBindsReferencedImagesWhenEditing(t *testing.T) {
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
	created, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Original", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(created) != 1 {
		t.Fatalf("CreateAnnouncement() = %#v, %v", created, err)
	}
	if _, err := service.PublishAnnouncement(context.Background(), application.PublishAnnouncementRequest{ActorID: 7, MessageID: created[0].ID}); err != nil {
		t.Fatalf("PublishAnnouncement() = %v", err)
	}
	files := &privateMessageFilesFake{}
	editService := application.NewService(application.Dependencies{
		Messages: repository, Categories: repository,
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{10}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
		Files:         files, Transactions: platformdatabase.NewTransactionRunner(db), Clock: application.ClockFunc(func() time.Time { return now }),
	})
	edited, err := editService.EditAnnouncement(context.Background(), application.EditAnnouncementRequest{ActorID: 7, MessageID: created[0].ID, Title: "Edited", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}, ImageIDs: []uint{92}})
	if err != nil {
		t.Fatalf("EditAnnouncement() = %v", err)
	}
	if files.binding.ActorID != 7 || files.binding.MessageLogicalID != edited.LogicalID || len(files.binding.ImageIDs) != 1 || files.binding.ImageIDs[0] != 92 {
		t.Fatalf("image binding = %#v", files.binding)
	}
}

func TestAnnouncementServiceRejectsEditingRevokedOrExpiredCopy(t *testing.T) {
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
	created, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{ActorID: 7, CategoryCode: "notice", Title: "Original", Markdown: "Body", PublishAt: &now, Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}})
	if err != nil || len(created) != 1 {
		t.Fatalf("CreateAnnouncement() = %#v, %v", created, err)
	}
	message := created[0]
	revokedAt := now.Add(time.Minute)
	message.Status = domain.MessageStatusRevoked
	message.RevokedAt = &revokedAt
	message.AggregateVersion = 2
	if _, err := repository.ChangeMessage(context.Background(), application.MessageChange{Message: message, Event: domain.MessageEvent{EventID: "announcement-revoked", EventName: domain.EventNameMessageRevoked, EventVersion: 1, MessageCopyID: message.ID, OrganizationID: 10, OccurredAt: revokedAt, AggregateVersion: 2}}); err != nil {
		t.Fatalf("revoke announcement: %v", err)
	}
	if _, err := service.EditAnnouncement(context.Background(), application.EditAnnouncementRequest{ActorID: 7, MessageID: message.ID, Title: "Edited", Markdown: "Body", Targets: []application.DynamicAudienceTarget{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}}); err != application.ErrMessageImmutable {
		t.Fatalf("EditAnnouncement revoked error = %v, want ErrMessageImmutable", err)
	}
}
