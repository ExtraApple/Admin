package domain_test

import (
	"errors"
	"testing"
	"time"

	"admin/internal/messaging/domain"
)

func TestMessageEventValidationAcceptsMinimalLifecycleEvent(t *testing.T) {
	event := domain.MessageEvent{
		EventID:          "event-1",
		EventName:        domain.EventNameMessagePublished,
		EventVersion:     1,
		MessageCopyID:    10,
		OrganizationID:   20,
		OccurredAt:       time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC),
		AggregateVersion: 3,
	}
	if err := domain.ValidateMessageEvent(event); err != nil {
		t.Fatalf("ValidateMessageEvent() error = %v", err)
	}
}

func TestMessageEventValidationRejectsUnknownNameAndMissingReference(t *testing.T) {
	event := domain.MessageEvent{EventName: "messaging.message.unknown.v1", EventVersion: 1}
	if err := domain.ValidateMessageEvent(event); !errors.Is(err, domain.ErrMessageEventInvalid) {
		t.Fatalf("ValidateMessageEvent() error = %v, want ErrMessageEventInvalid", err)
	}
}

func TestSnapshotLeaseFenceIsMonotonicAndOwnerProtected(t *testing.T) {
	now := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	consumption := domain.EventConsumption{ConsumerName: "websocket", EventID: "event-1"}
	if err := domain.ClaimSnapshotLease(&consumption, "worker-a", now, 30*time.Second); err != nil {
		t.Fatalf("first ClaimSnapshotLease() error = %v", err)
	}
	firstFence := consumption.SnapshotFence
	if firstFence == 0 {
		t.Fatal("first snapshot lease fence must be positive")
	}
	if err := domain.ClaimSnapshotLease(&consumption, "worker-b", now, 30*time.Second); !errors.Is(err, domain.ErrSnapshotLeaseHeld) {
		t.Fatalf("second ClaimSnapshotLease() error = %v, want ErrSnapshotLeaseHeld", err)
	}
	if err := domain.RenewSnapshotLease(&consumption, "worker-a", firstFence, now.Add(10*time.Second), 30*time.Second); err != nil {
		t.Fatalf("RenewSnapshotLease() error = %v", err)
	}
	if err := domain.CompleteSnapshot(&consumption, "worker-b", firstFence, now.Add(11*time.Second)); !errors.Is(err, domain.ErrSnapshotLeaseOwner) {
		t.Fatalf("CompleteSnapshot() error = %v, want ErrSnapshotLeaseOwner", err)
	}
	if err := domain.CompleteSnapshot(&consumption, "worker-a", firstFence, now.Add(11*time.Second)); err != nil {
		t.Fatalf("CompleteSnapshot() error = %v", err)
	}
}

func TestAudienceSnapshotRejectsCapacityWithoutPartialUsers(t *testing.T) {
	users := []uint{1, 2, 3}
	snapshot, err := domain.NewAudienceSnapshot("event-1", users, 2)
	if !errors.Is(err, domain.ErrAudienceCapacityExceeded) {
		t.Fatalf("NewAudienceSnapshot() error = %v, want ErrAudienceCapacityExceeded", err)
	}
	if snapshot.Users != nil {
		t.Fatalf("capacity failure retained partial users: %#v", snapshot.Users)
	}
	if snapshot.ObservedCount != len(users) {
		t.Fatalf("observed count = %d, want %d", snapshot.ObservedCount, len(users))
	}
}

func TestConsumerDLQReplayReopensSameProjectionAndIncrementsCycle(t *testing.T) {
	deadLetter := domain.ConsumerDeadLetter{ConsumerName: "websocket", EventID: "event-1", Status: domain.ConsumerDLQStatusPending}
	if err := domain.BeginConsumerDLQReplay(&deadLetter, "worker-a", time.Now().UTC(), 30*time.Second); err != nil {
		t.Fatalf("BeginConsumerDLQReplay() error = %v", err)
	}
	if err := domain.MarkConsumerDLQReplayed(&deadLetter, "worker-a", deadLetter.ReplayLeaseFence); err != nil {
		t.Fatalf("MarkConsumerDLQReplayed() error = %v", err)
	}
	if deadLetter.Status != domain.ConsumerDLQStatusReplayed || deadLetter.ReplayCycle != 1 {
		t.Fatalf("replayed DLQ = %+v", deadLetter)
	}
	if err := domain.ReopenConsumerDLQAfterFailure(&deadLetter, "failure-code"); err != nil {
		t.Fatalf("ReopenConsumerDLQAfterFailure() error = %v", err)
	}
	if deadLetter.Status != domain.ConsumerDLQStatusPending || deadLetter.ReplayCycle != 2 || deadLetter.LastFailureCode != "failure-code" {
		t.Fatalf("reopened DLQ = %+v", deadLetter)
	}
}
