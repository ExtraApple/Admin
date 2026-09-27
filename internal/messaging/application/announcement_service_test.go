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

	publishedCount, err := service.PublishDueAnnouncements(context.Background(), scheduledAt, 100, &application.AnnouncementScan{})
	if err != nil || publishedCount.Succeeded != 1 {
		t.Fatalf("PublishDueAnnouncements() = %+v, %v", publishedCount, err)
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
	publishedCount, err := service.PublishDueAnnouncements(context.Background(), scheduledAt, 100, &application.AnnouncementScan{})
	if err != nil || publishedCount.Succeeded != 1 {
		t.Fatalf("PublishDueAnnouncements() = %+v, %v", publishedCount, err)
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
	expiredCount, err := service.ExpireDueAnnouncements(context.Background(), expiresAt, 100, &application.AnnouncementScan{})
	if err != nil || expiredCount.Succeeded != 1 {
		t.Fatalf("ExpireDueAnnouncements() = %+v, %v", expiredCount, err)
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

func TestAnnouncementMaintenanceExpiresMissedWindowWithoutPublishing(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatal(err)
	}
	r := messaginggorm.NewRepository(db)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	publishAt, expiresAt := now.Add(-time.Hour), now
	message := messaginggorm.Message{LogicalID: "missed-window", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, Title: "Notice", BodyHTML: "<p>Body</p>", PublishAt: &publishAt, ExpiresAt: &expiresAt, AggregateVersion: 1}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	s := newAnnouncementService(r, db, now)
	if count, err := s.PublishDueAnnouncements(context.Background(), now, 100, &application.AnnouncementScan{}); err != nil || count.Processed != 0 {
		t.Fatalf("missed window published: %+v %v", count, err)
	}
	if count, err := s.ExpireDueAnnouncements(context.Background(), now, 100, &application.AnnouncementScan{}); err != nil || count.Succeeded != 1 {
		t.Fatalf("missed window expiry: %+v %v", count, err)
	}
	got, err := r.FindMessage(context.Background(), message.ID)
	if err != nil || got.Status != domain.MessageStatusExpired || got.AggregateVersion != 2 {
		t.Fatalf("expired copy: %+v %v", got, err)
	}
	var events []messaginggorm.MessageOutbox
	if err := db.Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventName != domain.EventNameMessageExpired {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestAnnouncementMaintenanceIsolatesOutboxFailurePerCopy(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatal(err)
	}
	r := messaginggorm.NewRepository(db)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	for _, org := range []uint{10, 20} {
		message := messaginggorm.Message{LogicalID: "notice", OrganizationID: org, SenderID: 7, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, Title: "Notice", BodyHTML: "<p>Body</p>", PublishAt: &now, AggregateVersion: 1}
		if err := db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("CREATE TRIGGER fail_first_outbox BEFORE INSERT ON message_outboxes WHEN NEW.message_copy_id = 1 BEGIN SELECT RAISE(ABORT, 'outbox unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	s := newAnnouncementService(r, db, now)
	result, err := s.PublishDueAnnouncements(context.Background(), now, 100, &application.AnnouncementScan{})
	if err != nil || result.Processed != 2 || result.Failed != 1 || result.Succeeded != 1 {
		t.Fatalf("isolation result: %+v %v", result, err)
	}
	first, err := r.FindMessage(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.FindMessage(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != domain.MessageStatusScheduled || first.AggregateVersion != 1 || second.Status != domain.MessageStatusPublished || second.AggregateVersion != 2 {
		t.Fatalf("copy isolation failed: first=%+v second=%+v", first, second)
	}
}

func TestAnnouncementMaintenanceCASConflictSkipsAndAdvancesScan(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatal(err)
	}
	r := messaginggorm.NewRepository(db)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	for range 2 {
		message := messaginggorm.Message{LogicalID: "notice", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, Title: "Notice", BodyHTML: "<p>Body</p>", PublishAt: &now, AggregateVersion: 1}
		if err := db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("CREATE TRIGGER conflict_first_copy BEFORE UPDATE ON messages WHEN OLD.id = 1 BEGIN SELECT RAISE(IGNORE); END").Error; err != nil {
		t.Fatal(err)
	}
	s := newAnnouncementService(r, db, now)
	scan := application.AnnouncementScan{}
	result, err := s.PublishDueAnnouncements(context.Background(), now, 1, &scan)
	if err != nil || result.Processed != 1 || result.Skipped != 1 || result.Succeeded != 0 || scan.AfterID != 1 {
		t.Fatalf("CAS outcome: %+v scan=%+v err=%v", result, scan, err)
	}
	result, err = s.PublishDueAnnouncements(context.Background(), now, 1, &scan)
	if err != nil || result.Succeeded != 1 || result.Items[0].MessageCopyID != 2 {
		t.Fatalf("later copy starved: %+v %v", result, err)
	}
	first, err := r.FindMessage(context.Background(), 1)
	if err != nil || first.Status != domain.MessageStatusScheduled {
		t.Fatalf("conflict treated as published: %+v %v", first, err)
	}
}

func TestAnnouncementMaintenanceTimeBoundariesAndManualExpiryRejection(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatal(err)
	}
	r := messaginggorm.NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Second), now.Add(-time.Hour)
	messages := []messaginggorm.Message{
		{PublishAt: &now, ExpiresAt: &future},
		{PublishAt: &future},
		{PublishAt: &past, ExpiresAt: &now},
	}
	for i := range messages {
		m := &messages[i]
		m.LogicalID = "boundary"
		m.OrganizationID = 10
		m.SenderID = 7
		m.Kind = domain.MessageKindAnnouncement
		m.Status = domain.MessageStatusScheduled
		m.Title = "Notice"
		m.BodyHTML = "<p>Body</p>"
		m.AggregateVersion = 1
		if err := db.Create(m).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := newAnnouncementService(r, db, now)
	if _, err := s.PublishAnnouncement(ctx, application.PublishAnnouncementRequest{ActorID: 7, MessageID: messages[2].ID}); err != application.ErrMessageImmutable {
		t.Fatalf("manual expired publish: %v", err)
	}
	result, err := s.PublishDueAnnouncements(ctx, now, 100, &application.AnnouncementScan{})
	if err != nil || result.Succeeded != 1 || result.Items[0].MessageCopyID != messages[0].ID {
		t.Fatalf("publish boundary: %+v %v", result, err)
	}
	result, err = s.ExpireDueAnnouncements(ctx, now, 100, &application.AnnouncementScan{})
	if err != nil || result.Succeeded != 1 || result.Items[0].MessageCopyID != messages[2].ID {
		t.Fatalf("expiry boundary: %+v %v", result, err)
	}
}

func TestAnnouncementMaintenanceDoesNotRepublishDeadOutbox(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatal(err)
	}
	r := messaginggorm.NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	m := messaginggorm.Message{LogicalID: "dead-announcement", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, Title: "Notice", BodyHTML: "<p>Body</p>", PublishAt: &now, AggregateVersion: 1}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	s := newAnnouncementService(r, db, now)
	if result, err := s.PublishDueAnnouncements(ctx, now, 10, &application.AnnouncementScan{}); err != nil || result.Succeeded != 1 {
		t.Fatalf("publish: %+v %v", result, err)
	}
	worker := application.NewOutboxWorker(application.OutboxWorkerConfig{Store: r, Publisher: &outboxPublisherFake{err: context.DeadlineExceeded}, WorkerID: "probe", Clock: application.ClockFunc(func() time.Time { return now })})
	for range 5 {
		if _, err := worker.RunOnce(ctx, 1); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
	}
	dead, total, err := r.ListOutboxes(ctx, application.OutboxListQuery{Statuses: []domain.OutboxStatus{domain.OutboxStatusDead}, Limit: 10})
	if err != nil || total != 1 || len(dead) != 1 {
		t.Fatalf("dead Outbox: %+v %d %v", dead, total, err)
	}
	if result, err := s.PublishDueAnnouncements(ctx, now, 10, &application.AnnouncementScan{}); err != nil || result.Processed != 0 {
		t.Fatalf("republished dead event: %+v %v", result, err)
	}
	copy, err := r.FindMessage(ctx, m.ID)
	if err != nil || copy.Status != domain.MessageStatusPublished || copy.AggregateVersion != 2 {
		t.Fatalf("delivery rolled back business state: %+v %v", copy, err)
	}
	if ok, err := r.ReplayOutbox(ctx, dead[0].ID); err != nil || !ok {
		t.Fatalf("replay: %v %v", ok, err)
	}
	pending, total, err := r.ListOutboxes(ctx, application.OutboxListQuery{Statuses: []domain.OutboxStatus{domain.OutboxStatusPending}, Limit: 10})
	if err != nil || total != 1 || pending[0].Event.EventID != dead[0].Event.EventID || pending[0].Event.AggregateVersion != 2 {
		t.Fatalf("replay replaced original event: %+v %v", pending, err)
	}
}
