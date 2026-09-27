package gormadapter_test

import (
	"context"
	"testing"
	"time"

	adapter "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

func TestDueAnnouncementCandidatesSeparatePublishAndExpireWindows(t *testing.T) {
	r, db := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	records := []adapter.Message{
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, PublishAt: &now},
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, PublishAt: &past, ExpiresAt: &now},
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusPublished, PublishAt: &past, ExpiresAt: &now},
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, PublishAt: &future},
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusDraft, PublishAt: &past, ExpiresAt: &now},
		{Kind: domain.MessageKindBroadcast, Status: domain.MessageStatusPublished, PublishAt: &past, ExpiresAt: &now},
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, ExpiresAt: &now},
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, PublishAt: &past, ExpiresAt: &future},
		{Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusRevoked, PublishAt: &past, ExpiresAt: &now},
	}
	for i := range records {
		records[i].OrganizationID = uint(i%2 + 1)
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	upper, err := r.AnnouncementCleanupUpperID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	query := application.DueAnnouncementQuery{Now: now, UpperID: upper, Limit: 100}
	got, err := r.FindDueAnnouncements(ctx, query)
	if err != nil || len(got) != 2 || got[0].ID != records[0].ID || got[1].ID != records[7].ID {
		t.Fatalf("publish candidates = %+v, error = %v", got, err)
	}
	query.Expiring = true
	got, err = r.FindDueAnnouncements(ctx, query)
	if err != nil || len(got) != 2 || got[0].ID != records[1].ID || got[1].ID != records[2].ID {
		t.Fatalf("expiry candidates = %+v, error = %v", got, err)
	}
}

func TestDueAnnouncementScanIsBoundedWithoutChangingManagementPagination(t *testing.T) {
	r, db := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	older := now.Add(-time.Hour)
	records := []adapter.Message{
		{OrganizationID: 1, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, PublishAt: &older},
		{OrganizationID: 1, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, PublishAt: &now},
		{OrganizationID: 1, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusScheduled, PublishAt: &now},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	upper, err := r.AnnouncementCleanupUpperID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inserted := records[0]
	inserted.ID = 0
	if err := db.Create(&inserted).Error; err != nil {
		t.Fatal(err)
	}
	query := application.DueAnnouncementQuery{Now: now, UpperID: upper, Limit: 1}
	for _, expected := range records {
		page, err := r.FindDueAnnouncements(ctx, query)
		if err != nil || len(page) != 1 || page[0].ID != expected.ID {
			t.Fatalf("bounded page = %+v, error = %v", page, err)
		}
		query.AfterID = page[0].ID
	}
	page, err := r.FindDueAnnouncements(ctx, query)
	if err != nil || len(page) != 0 {
		t.Fatalf("scan crossed fixed upper bound: %+v %v", page, err)
	}
	managed, total, err := r.ListMessages(ctx, application.MessageListQuery{OrganizationIDs: []uint{1}, Offset: 1, Limit: 1})
	if err != nil || total != 4 || len(managed) != 1 || managed[0].ID != records[1].ID {
		t.Fatalf("management ordering/pagination changed: %+v total=%d err=%v", managed, total, err)
	}
	for _, invalid := range []int{0, -1, 1001} {
		query.Limit = invalid
		if _, err := r.FindDueAnnouncements(ctx, query); err == nil {
			t.Fatalf("unbounded limit %d accepted", invalid)
		}
	}
	query.Limit = 1
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.FindDueAnnouncements(canceled, query); err == nil {
		t.Fatal("canceled query hidden as empty success")
	}
}
