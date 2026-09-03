package gormadapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
)

func TestRepositoryPersistsMessageRelationsAndCategoryConstraints(t *testing.T) {
	repository, db := openRepository(t)
	ctx := context.Background()
	category, err := repository.CreateCategory(ctx, domain.MessageCategory{OrganizationID: 10, Code: "notice", Name: "通知", Enabled: true})
	if err != nil {
		t.Fatalf("CreateCategory() = %v", err)
	}
	if _, err := repository.CreateCategory(ctx, domain.MessageCategory{OrganizationID: 10, Code: "notice", Name: "重复", Enabled: true}); err == nil {
		t.Fatal("CreateCategory() accepted duplicate organization code")
	}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	message, err := repository.PersistMessage(ctx, application.MessagePersistence{
		Message:   newMessage("logical-private", 10, 7, category.ID, domain.MessageKindPrivate, now),
		Recipient: &domain.PrivateRecipient{SenderID: 7, RecipientID: 8},
		Event:     event("event-private", 0, 10, 1, now),
	})
	if err != nil {
		t.Fatalf("PersistMessage() = %v", err)
	}
	if message.ID == 0 || message.AggregateVersion != 1 {
		t.Fatalf("persisted message = %#v", message)
	}
	var recipient messaginggorm.MessageRecipient
	if err := db.First(&recipient, "message_id = ? AND recipient_id = ?", message.ID, 8).Error; err != nil {
		t.Fatalf("load recipient relation: %v", err)
	}
	if recipient.ReadAt != nil || recipient.DeletedAt != nil {
		t.Fatalf("recipient state = %#v", recipient)
	}
	userIDs, err := repository.ListPrivateNotificationUserIDs(ctx, message.ID)
	if err != nil || len(userIDs) != 1 || userIDs[0] != 8 {
		t.Fatalf("ListPrivateNotificationUserIDs() = %#v, %v", userIDs, err)
	}
	var outbox messaginggorm.MessageOutbox
	if err := db.First(&outbox, "event_id = ?", "event-private").Error; err != nil {
		t.Fatalf("load outbox: %v", err)
	}
	if outbox.MessageCopyID != message.ID || outbox.Status != domain.OutboxStatusPending {
		t.Fatalf("outbox = %#v", outbox)
	}
	deleted, err := repository.DeleteCategoryIfUnused(ctx, category.ID)
	if err != nil || deleted {
		t.Fatalf("DeleteCategoryIfUnused() = %t, %v", deleted, err)
	}
}

func TestRepositoryInitializesDynamicInboxStatesFromMembershipTime(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	publishedAt := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	if _, err := repository.PersistMessage(ctx, application.MessagePersistence{
		Message:   newMessage("logical-broadcast", 10, 7, 0, domain.MessageKindBroadcast, publishedAt),
		Audiences: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeAll}},
		Event:     event("event-broadcast", 0, 10, 1, publishedAt),
	}); err != nil {
		t.Fatalf("persist broadcast: %v", err)
	}
	if _, err := repository.PersistMessage(ctx, application.MessagePersistence{
		Message:   newMessage("logical-announcement", 10, 7, 0, domain.MessageKindAnnouncement, publishedAt),
		Audiences: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeAll}},
		Event:     event("event-announcement", 0, 10, 1, publishedAt),
	}); err != nil {
		t.Fatalf("persist announcement: %v", err)
	}
	inbox, total, err := repository.ListInbox(ctx, application.InboxQuery{UserID: 8, Memberships: []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: publishedAt.Add(time.Hour)}}, Now: publishedAt.Add(2 * time.Hour)})
	if err != nil || total != 2 || len(inbox) != 2 {
		t.Fatalf("ListInbox() = %#v, total=%d, err=%v", inbox, total, err)
	}
	for _, item := range inbox {
		switch item.Message.Kind {
		case domain.MessageKindAnnouncement:
			if item.ReadAt == nil {
				t.Fatalf("historical announcement state = %#v, want read", item)
			}
		case domain.MessageKindBroadcast:
			if item.ReadAt != nil {
				t.Fatalf("broadcast state = %#v, want unread", item)
			}
		}
	}
	counts, err := repository.CountUnreadInbox(ctx, application.InboxQuery{UserID: 8, Memberships: []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: publishedAt.Add(time.Hour)}}, Now: publishedAt.Add(2 * time.Hour)})
	if err != nil || counts.Broadcast != 1 || counts.Announcement != 0 || counts.Total != 1 {
		t.Fatalf("CountUnreadInbox() = %#v, %v", counts, err)
	}
}

func TestRepositoryClaimsOnlyNextOutboxVersionAndRecoversExpiredLease(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	message, err := repository.PersistMessage(ctx, application.MessagePersistence{
		Message: newMessage("logical-outbox", 10, 7, 0, domain.MessageKindBroadcast, now),
		Event:   event("event-v1", 0, 10, 1, now),
	})
	if err != nil {
		t.Fatalf("persist v1: %v", err)
	}
	changed := message
	changed.Title = "changed"
	changed.AggregateVersion = 2
	if _, err := repository.ChangeMessage(ctx, application.MessageChange{Message: changed, Event: *event("event-v2", message.ID, 10, 2, now.Add(time.Second))}); err != nil {
		t.Fatalf("ChangeMessage() = %v", err)
	}
	claimed, err := repository.ClaimOutbox(ctx, application.OutboxClaim{WorkerID: "worker-a", Now: now, Lease: time.Minute, Limit: 10})
	if err != nil || len(claimed) != 1 || claimed[0].Event.EventID != "event-v1" {
		t.Fatalf("first ClaimOutbox() = %#v, %v", claimed, err)
	}
	if other, err := repository.ClaimOutbox(ctx, application.OutboxClaim{WorkerID: "worker-b", Now: now.Add(time.Second), Lease: time.Minute, Limit: 10}); err != nil || len(other) != 0 {
		t.Fatalf("concurrent ClaimOutbox() = %#v, %v", other, err)
	}
	if ok, err := repository.MarkOutboxPublished(ctx, application.OutboxLease{ID: claimed[0].ID, WorkerID: "worker-a", Now: now.Add(2 * time.Second)}); err != nil || !ok {
		t.Fatalf("MarkOutboxPublished() = %t, %v", ok, err)
	}
	claimed, err = repository.ClaimOutbox(ctx, application.OutboxClaim{WorkerID: "worker-b", Now: now.Add(3 * time.Second), Lease: time.Second, Limit: 10})
	if err != nil || len(claimed) != 1 || claimed[0].Event.EventID != "event-v2" {
		t.Fatalf("second ClaimOutbox() = %#v, %v", claimed, err)
	}
	recovered, err := repository.ClaimOutbox(ctx, application.OutboxClaim{WorkerID: "worker-c", Now: now.Add(5 * time.Second), Lease: time.Minute, Limit: 10})
	if err != nil || len(recovered) != 1 || recovered[0].ID != claimed[0].ID || recovered[0].WorkerID != "worker-c" {
		t.Fatalf("expired ClaimOutbox() = %#v, %v", recovered, err)
	}
}

func TestRepositoryFencesAudienceBatchesAndSupersedesOlderConsumerEvent(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	newer := *event("event-new", 42, 10, 2, now)
	claim, acquired, err := repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: newer, WorkerID: "worker-a", Now: now, Lease: time.Minute})
	if err != nil || !acquired || claim.SnapshotFence == 0 {
		t.Fatalf("ClaimEventConsumption() = %#v, acquired=%t, err=%v", claim, acquired, err)
	}
	users := make([]uint, 501)
	for index := range users {
		users[index] = uint(index + 1)
	}
	if ok, err := repository.PersistAudienceDeliveryBatch(ctx, application.AudienceDeliveryBatch{EventConsumptionLease: application.EventConsumptionLease{ConsumerName: "websocket", EventID: newer.EventID, WorkerID: "worker-a", SnapshotFence: claim.SnapshotFence, Now: now}, MessageCopyID: newer.MessageCopyID, UserIDs: users, ExpiresAt: now.Add(24 * time.Hour)}); ok || !errors.Is(err, application.ErrAudienceBatchTooLarge) {
		t.Fatalf("oversized PersistAudienceDeliveryBatch() = %t, %v", ok, err)
	}
	if ok, err := repository.PersistAudienceDeliveryBatch(ctx, application.AudienceDeliveryBatch{EventConsumptionLease: application.EventConsumptionLease{ConsumerName: "websocket", EventID: newer.EventID, WorkerID: "worker-a", SnapshotFence: claim.SnapshotFence, Now: now}, MessageCopyID: newer.MessageCopyID, UserIDs: []uint{7, 8}, ExpiresAt: now.Add(24 * time.Hour)}); err != nil || !ok {
		t.Fatalf("PersistAudienceDeliveryBatch() = %t, %v", ok, err)
	}
	finalized, err := repository.FinalizeEventConsumption(ctx, application.EventConsumptionCompletion{EventConsumptionLease: application.EventConsumptionLease{ConsumerName: "websocket", EventID: newer.EventID, WorkerID: "worker-a", SnapshotFence: claim.SnapshotFence, Now: now}, MessageCopyID: newer.MessageCopyID, AggregateVersion: newer.AggregateVersion})
	if err != nil || finalized.Status != domain.EventConsumptionStatusCompleted {
		t.Fatalf("FinalizeEventConsumption() = %#v, %v", finalized, err)
	}
	older := *event("event-old", 42, 10, 1, now.Add(time.Second))
	claim, acquired, err = repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: older, WorkerID: "worker-b", Now: now.Add(2 * time.Second), Lease: time.Minute})
	if err != nil || !acquired {
		t.Fatalf("claim older event = %#v, acquired=%t, err=%v", claim, acquired, err)
	}
	finalized, err = repository.FinalizeEventConsumption(ctx, application.EventConsumptionCompletion{EventConsumptionLease: application.EventConsumptionLease{ConsumerName: "websocket", EventID: older.EventID, WorkerID: "worker-b", SnapshotFence: claim.SnapshotFence, Now: now.Add(2 * time.Second)}, MessageCopyID: older.MessageCopyID, AggregateVersion: older.AggregateVersion})
	if err != nil || finalized.Status != domain.EventConsumptionStatusSuperseded {
		t.Fatalf("finalize older event = %#v, %v", finalized, err)
	}
}

func TestRepositoryReopensReplayedConsumerDeadLetter(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	input := application.ConsumerDeadLetterInput{ConsumerName: "websocket", Event: *event("event-dlq", 42, 10, 1, now), OriginalQueue: "admin.messaging.websocket.dlq", RetryAttempt: 5, FailureCode: "MSG_RETRY_EXHAUSTED", Now: now}
	deadLetter, err := repository.RecordConsumerDeadLetter(ctx, input)
	if err != nil || deadLetter.Status != domain.ConsumerDLQStatusPending {
		t.Fatalf("RecordConsumerDeadLetter() = %#v, %v", deadLetter, err)
	}
	deadLetter, acquired, err := repository.ClaimConsumerDeadLetterReplay(ctx, application.ConsumerDeadLetterReplayClaim{ID: deadLetter.ID, WorkerID: "worker-a", Now: now, Lease: time.Minute})
	if err != nil || !acquired || deadLetter.Status != domain.ConsumerDLQStatusReplaying {
		t.Fatalf("ClaimConsumerDeadLetterReplay() = %#v, %t, %v", deadLetter, acquired, err)
	}
	if ok, err := repository.MarkConsumerDeadLetterReplayed(ctx, application.ConsumerDeadLetterReplayResult{ID: deadLetter.ID, WorkerID: "worker-a", Fence: deadLetter.ReplayLeaseFence, Now: now.Add(time.Second)}); err != nil || !ok {
		t.Fatalf("MarkConsumerDeadLetterReplayed() = %t, %v", ok, err)
	}
	input.Now = now.Add(2 * time.Second)
	input.FailureCode = "MSG_RETRY_EXHAUSTED_AGAIN"
	deadLetter, err = repository.RecordConsumerDeadLetter(ctx, input)
	if err != nil || deadLetter.Status != domain.ConsumerDLQStatusPending || deadLetter.ReplayCycle < 2 || deadLetter.LastFailureCode != input.FailureCode || deadLetter.Event.OccurredAt != input.Event.OccurredAt {
		t.Fatalf("reopened dead letter = %#v, %v", deadLetter, err)
	}
}

func openRepository(t *testing.T) (*messaginggorm.Repository, *gorm.DB) {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate messaging models: %v", err)
	}
	return messaginggorm.NewRepository(db), db
}

func newMessage(logicalID string, organizationID, senderID, categoryID uint, kind domain.MessageKind, publishedAt time.Time) domain.Message {
	return domain.Message{LogicalID: logicalID, OrganizationID: organizationID, SenderID: senderID, CategoryID: categoryID, Kind: kind, Status: domain.MessageStatusPublished, Title: "title", BodyHTML: "<p>body</p>", PublishAt: &publishedAt}
}

func event(eventID string, messageCopyID, organizationID uint, aggregateVersion uint64, occurredAt time.Time) *domain.MessageEvent {
	return &domain.MessageEvent{EventID: eventID, EventName: domain.EventNameMessageCreated, EventVersion: 1, MessageCopyID: messageCopyID, OrganizationID: organizationID, OccurredAt: occurredAt, AggregateVersion: aggregateVersion}
}

func TestRepositoryFiltersPaginatesAndFencesConsumerLease(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	for index, title := range []string{"alpha", "beta"} {
		message := newMessage("logical-filter-"+title, 10, 7, 0, domain.MessageKindBroadcast, now.Add(time.Duration(index)*time.Minute))
		message.Title = title
		if _, err := repository.PersistMessage(ctx, application.MessagePersistence{Message: message, Audiences: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeAll}}, Event: event("event-filter-"+title, 0, 10, 1, now.Add(time.Duration(index)*time.Minute))}); err != nil {
			t.Fatalf("persist %s: %v", title, err)
		}
	}
	inbox, total, err := repository.ListInbox(ctx, application.InboxQuery{UserID: 8, Memberships: []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}, Kinds: []domain.MessageKind{domain.MessageKindBroadcast}, Keyword: "alpha", Now: now.Add(2 * time.Minute), Limit: 1})
	if err != nil || total != 1 || len(inbox) != 1 || inbox[0].Message.Title != "alpha" {
		t.Fatalf("filtered inbox = %#v, total=%d, err=%v", inbox, total, err)
	}
	event := *event("event-fence", 99, 10, 1, now)
	claim, acquired, err := repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: event, WorkerID: "worker-a", Now: now, Lease: time.Minute})
	if err != nil || !acquired {
		t.Fatalf("claim consumption = %#v, %t, %v", claim, acquired, err)
	}
	if renewed, err := repository.RenewEventConsumptionLease(ctx, application.EventConsumptionLease{ConsumerName: "websocket", EventID: event.EventID, WorkerID: "worker-a", SnapshotFence: claim.SnapshotFence, Now: now.Add(time.Second), Lease: time.Minute}); err != nil || !renewed {
		t.Fatalf("RenewEventConsumptionLease() = %t, %v", renewed, err)
	}
	claim, acquired, err = repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: event, WorkerID: "worker-a", Now: now.Add(2 * time.Second), Lease: time.Minute})
	if err != nil || !acquired || claim.SnapshotFence != 2 {
		t.Fatalf("reclaim consumption = %#v, %t, %v", claim, acquired, err)
	}
	if ok, err := repository.PersistAudienceDeliveryBatch(ctx, application.AudienceDeliveryBatch{EventConsumptionLease: application.EventConsumptionLease{ConsumerName: "websocket", EventID: event.EventID, WorkerID: "worker-a", SnapshotFence: 1, Now: now.Add(2 * time.Second)}, MessageCopyID: event.MessageCopyID, UserIDs: []uint{7}, ExpiresAt: now.Add(time.Hour)}); err != nil || ok {
		t.Fatalf("stale fenced batch = %t, %v", ok, err)
	}
}

func TestRepositoryRetainsLinkedAudienceSnapshotUntilDeadLetterFinalization(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := *event("event-retain", 77, 10, 1, now)
	claim, acquired, err := repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: event, WorkerID: "worker-a", Now: now, Lease: time.Minute})
	if err != nil || !acquired {
		t.Fatalf("claim snapshot = %#v, %t, %v", claim, acquired, err)
	}
	if ok, err := repository.PersistAudienceDeliveryBatch(ctx, application.AudienceDeliveryBatch{EventConsumptionLease: application.EventConsumptionLease{ConsumerName: "websocket", EventID: event.EventID, WorkerID: "worker-a", SnapshotFence: claim.SnapshotFence, Now: now}, MessageCopyID: event.MessageCopyID, UserIDs: []uint{7}, ExpiresAt: now.Add(24 * time.Hour)}); err != nil || !ok {
		t.Fatalf("persist snapshot = %t, %v", ok, err)
	}
	deadLetter, err := repository.RecordConsumerDeadLetter(ctx, application.ConsumerDeadLetterInput{ConsumerName: "websocket", Event: event, OriginalQueue: "websocket.dlq", RetryAttempt: 5, FailureCode: "MSG_RETRY_EXHAUSTED", Now: now})
	if err != nil {
		t.Fatalf("record dead letter: %v", err)
	}
	if removed, err := repository.CleanupAudienceDeliveries(ctx, now.Add(48*time.Hour), 10); err != nil || removed != 0 {
		t.Fatalf("early snapshot cleanup = %d, %v", removed, err)
	}
	deadLetter, acquired, err = repository.ClaimConsumerDeadLetterReplay(ctx, application.ConsumerDeadLetterReplayClaim{ID: deadLetter.ID, WorkerID: "worker-a", Now: now.Add(time.Minute), Lease: time.Minute})
	if err != nil || !acquired {
		t.Fatalf("claim dead letter replay = %#v, %t, %v", deadLetter, acquired, err)
	}
	if ok, err := repository.MarkConsumerDeadLetterReplayed(ctx, application.ConsumerDeadLetterReplayResult{ID: deadLetter.ID, WorkerID: "worker-a", Fence: deadLetter.ReplayLeaseFence, Now: now.Add(time.Minute + time.Second)}); err != nil || !ok {
		t.Fatalf("mark replayed = %t, %v", ok, err)
	}
	if removed, err := repository.CleanupAudienceDeliveries(ctx, now.Add(30*24*time.Hour+3*time.Minute), 10); err != nil || removed != 1 {
		t.Fatalf("final snapshot cleanup = %d, %v", removed, err)
	}
	if removed, err := repository.CleanupFinalConsumerDeadLetters(ctx, now.Add(30*24*time.Hour+3*time.Minute), 10); err != nil || removed != 1 {
		t.Fatalf("final dead letter cleanup = %d, %v", removed, err)
	}
}

func TestRepositoryStoresInvalidConsumerDeadLetterWithoutPayloadAndRejectsReplay(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	deadLetter, err := repository.RecordConsumerDeadLetter(ctx, application.ConsumerDeadLetterInput{ConsumerName: "websocket", OriginalQueue: "admin.messaging.websocket", RetryAttempt: 5, FailureCode: "message_event_invalid", Invalid: true, Fingerprint: fingerprint, Now: now})
	if err != nil || !deadLetter.Invalid || deadLetter.Fingerprint != fingerprint || deadLetter.Replayable {
		t.Fatalf("invalid dead letter = %#v, err=%v", deadLetter, err)
	}
	items, total, err := repository.ListConsumerDeadLetters(ctx, application.ConsumerDeadLetterListQuery{ConsumerName: "websocket"})
	if err != nil || total != 1 || len(items) != 1 || items[0].Fingerprint != fingerprint || items[0].Event.EventID == "" {
		t.Fatalf("listed invalid dead letters = %#v total=%d err=%v", items, total, err)
	}
	if _, acquired, err := repository.ClaimConsumerDeadLetterReplay(ctx, application.ConsumerDeadLetterReplayClaim{ID: deadLetter.ID, WorkerID: "worker-a", Now: now, Lease: time.Minute}); err == nil || acquired {
		t.Fatalf("invalid dead-letter replay claim = acquired:%t err:%v, want rejection", acquired, err)
	}
}

func TestRepositoryReusesOnlyMarkedCompleteAudienceSnapshots(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	messageEvent := *event("event-snapshot", 77, 10, 1, now)
	claim, acquired, err := repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: messageEvent, WorkerID: "worker-a", Now: now, Lease: time.Minute})
	if err != nil || !acquired {
		t.Fatalf("claim snapshot = %#v, %t, %v", claim, acquired, err)
	}
	lease := application.EventConsumptionLease{ConsumerName: "websocket", EventID: messageEvent.EventID, WorkerID: "worker-a", SnapshotFence: claim.SnapshotFence, Now: now, Lease: time.Minute}
	if reset, err := repository.ResetIncompleteAudienceSnapshot(ctx, lease); err != nil || !reset {
		t.Fatalf("ResetIncompleteAudienceSnapshot() = %t, %v", reset, err)
	}
	if ok, err := repository.PersistAudienceDeliveryBatch(ctx, application.AudienceDeliveryBatch{EventConsumptionLease: lease, MessageCopyID: messageEvent.MessageCopyID, UserIDs: []uint{7, 8}, ExpiresAt: now.Add(24 * time.Hour)}); err != nil || !ok {
		t.Fatalf("persist snapshot = %t, %v", ok, err)
	}
	if complete, err := repository.MarkAudienceSnapshotComplete(ctx, lease); err != nil || !complete {
		t.Fatalf("MarkAudienceSnapshotComplete() = %t, %v", complete, err)
	}
	claim, acquired, err = repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: messageEvent, WorkerID: "worker-a", Now: now.Add(time.Second), Lease: time.Minute})
	if err != nil || !acquired || !claim.SnapshotComplete {
		t.Fatalf("reclaim complete snapshot = %#v, %t, %v", claim, acquired, err)
	}
	userIDs, err := repository.ListAudienceDeliveryUserIDs(ctx, "websocket", messageEvent.EventID)
	if err != nil || len(userIDs) != 2 || userIDs[0] != 7 || userIDs[1] != 8 {
		t.Fatalf("complete snapshot users = %#v, %v", userIDs, err)
	}

	incomplete := *event("event-incomplete", 78, 10, 1, now)
	claim, acquired, err = repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: incomplete, WorkerID: "worker-a", Now: now, Lease: time.Second})
	if err != nil || !acquired {
		t.Fatalf("claim incomplete snapshot = %#v, %t, %v", claim, acquired, err)
	}
	lease = application.EventConsumptionLease{ConsumerName: "websocket", EventID: incomplete.EventID, WorkerID: "worker-a", SnapshotFence: claim.SnapshotFence, Now: now, Lease: time.Second}
	if reset, err := repository.ResetIncompleteAudienceSnapshot(ctx, lease); err != nil || !reset {
		t.Fatalf("initial incomplete reset = %t, %v", reset, err)
	}
	if ok, err := repository.PersistAudienceDeliveryBatch(ctx, application.AudienceDeliveryBatch{EventConsumptionLease: lease, MessageCopyID: incomplete.MessageCopyID, UserIDs: []uint{9}, ExpiresAt: now.Add(24 * time.Hour)}); err != nil || !ok {
		t.Fatalf("persist partial snapshot = %t, %v", ok, err)
	}
	if discarded, err := repository.DiscardAudienceDeliverySnapshot(ctx, lease); err != nil || !discarded {
		t.Fatalf("DiscardAudienceDeliverySnapshot() = %t, %v", discarded, err)
	}
	userIDs, err = repository.ListAudienceDeliveryUserIDs(ctx, "websocket", incomplete.EventID)
	if err != nil || len(userIDs) != 0 {
		t.Fatalf("discarded incomplete snapshot users = %#v, %v", userIDs, err)
	}
	claim, acquired, err = repository.ClaimEventConsumption(ctx, application.EventConsumptionClaim{ConsumerName: "websocket", Event: incomplete, WorkerID: "worker-b", Now: now.Add(2 * time.Second), Lease: time.Minute})
	if err != nil || !acquired || claim.SnapshotComplete {
		t.Fatalf("reclaim incomplete snapshot = %#v, %t, %v", claim, acquired, err)
	}
	lease = application.EventConsumptionLease{ConsumerName: "websocket", EventID: incomplete.EventID, WorkerID: "worker-b", SnapshotFence: claim.SnapshotFence, Now: now.Add(2 * time.Second), Lease: time.Minute}
	if reset, err := repository.ResetIncompleteAudienceSnapshot(ctx, lease); err != nil || !reset {
		t.Fatalf("retry incomplete reset = %t, %v", reset, err)
	}
	userIDs, err = repository.ListAudienceDeliveryUserIDs(ctx, "websocket", incomplete.EventID)
	if err != nil || len(userIDs) != 0 {
		t.Fatalf("incomplete snapshot users = %#v, %v", userIDs, err)
	}
}
func TestRepositoryHidesExpiredAndNoLongerEligibleDynamicMessages(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	expiresAt := now.Add(-time.Minute)
	message, err := repository.PersistMessage(ctx, application.MessagePersistence{Message: func() domain.Message {
		value := newMessage("logical-expired", 10, 7, 0, domain.MessageKindBroadcast, now.Add(-time.Hour))
		value.ExpiresAt = &expiresAt
		return value
	}(), Audiences: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeAll}}, Event: event("event-expired", 0, 10, 1, now.Add(-time.Hour))})
	if err != nil {
		t.Fatalf("persist expired message: %v", err)
	}
	visible, total, err := repository.ListInbox(ctx, application.InboxQuery{UserID: 8, Memberships: []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}, Now: now})
	if err != nil || total != 0 || len(visible) != 0 {
		t.Fatalf("expired inbox = %#v total=%d err=%v", visible, total, err)
	}
	if message.ID == 0 {
		t.Fatal("persisted message ID is zero")
	}
	visible, total, err = repository.ListInbox(ctx, application.InboxQuery{UserID: 8, Now: now})
	if err != nil || total != 0 || len(visible) != 0 {
		t.Fatalf("inbox without current membership = %#v total=%d err=%v", visible, total, err)
	}
}
