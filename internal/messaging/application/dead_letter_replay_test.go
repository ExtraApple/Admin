package application_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type consumerDeadLetterReplayStoreFake struct {
	claimed      application.ConsumerDeadLetter
	acquired     bool
	claim        application.ConsumerDeadLetterReplayClaim
	marked       application.ConsumerDeadLetterReplayResult
	returned     application.ConsumerDeadLetterReplayResult
	returnCalled bool
}

func (store *consumerDeadLetterReplayStoreFake) RecordConsumerDeadLetter(context.Context, application.ConsumerDeadLetterInput) (application.ConsumerDeadLetter, error) {
	panic("unused")
}
func (store *consumerDeadLetterReplayStoreFake) ListConsumerDeadLetters(context.Context, application.ConsumerDeadLetterListQuery) ([]application.ConsumerDeadLetter, int64, error) {
	panic("unused")
}
func (store *consumerDeadLetterReplayStoreFake) ClaimConsumerDeadLetterReplay(_ context.Context, claim application.ConsumerDeadLetterReplayClaim) (application.ConsumerDeadLetter, bool, error) {
	store.claim = claim
	return store.claimed, store.acquired, nil
}
func (store *consumerDeadLetterReplayStoreFake) MarkConsumerDeadLetterReplayed(_ context.Context, result application.ConsumerDeadLetterReplayResult) (bool, error) {
	store.marked = result
	return true, nil
}
func (store *consumerDeadLetterReplayStoreFake) ReturnConsumerDeadLetterPending(_ context.Context, result application.ConsumerDeadLetterReplayResult) (bool, error) {
	store.returnCalled = true
	store.returned = result
	return true, nil
}
func (store *consumerDeadLetterReplayStoreFake) DiscardConsumerDeadLetter(context.Context, uint, time.Time) (bool, error) {
	panic("unused")
}
func (store *consumerDeadLetterReplayStoreFake) CleanupFinalConsumerDeadLetters(context.Context, time.Time, int) (int64, error) {
	panic("unused")
}

type consumerDeadLetterReplayPublisherFake struct {
	consumerName string
	event        domain.MessageEvent
	err          error
}

func (publisher *consumerDeadLetterReplayPublisherFake) PublishConsumerReplay(_ context.Context, consumerName string, event domain.MessageEvent) error {
	publisher.consumerName = consumerName
	publisher.event = event
	return publisher.err
}

func TestConsumerDeadLetterReplayClaimsThirtySecondLeasePublishesThenFinalizes(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-replay", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 2}
	store := &consumerDeadLetterReplayStoreFake{claimed: application.ConsumerDeadLetter{ID: 7, ConsumerName: "websocket", Event: event, ReplayLeaseFence: 3}, acquired: true}
	publisher := &consumerDeadLetterReplayPublisherFake{}
	service := application.NewConsumerDeadLetterReplayService(application.ConsumerDeadLetterReplayConfig{Store: store, Publisher: publisher, WorkerID: "worker-a", Clock: application.ClockFunc(func() time.Time { return now })})
	result, replayed, err := service.Replay(context.Background(), 7)
	if err != nil || !replayed || result.ID != 7 || store.claim.ID != 7 || store.claim.WorkerID != "worker-a" || store.claim.Lease != 30*time.Second || publisher.consumerName != "websocket" || publisher.event != event || store.marked.ID != 7 || store.marked.WorkerID != "worker-a" || store.marked.Fence != 3 || store.returnCalled {
		t.Fatalf("Replay() result=%#v replayed=%t err=%v claim=%#v published=%#v marked=%#v returned=%#v", result, replayed, err, store.claim, publisher, store.marked, store.returned)
	}
}

func TestConsumerDeadLetterReplayReturnsClaimToPendingAfterPublishFailure(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	event := domain.MessageEvent{EventID: "event-replay-fail", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 2}
	store := &consumerDeadLetterReplayStoreFake{claimed: application.ConsumerDeadLetter{ID: 8, ConsumerName: "websocket", Event: event, ReplayLeaseFence: 4}, acquired: true}
	service := application.NewConsumerDeadLetterReplayService(application.ConsumerDeadLetterReplayConfig{Store: store, Publisher: &consumerDeadLetterReplayPublisherFake{err: context.DeadlineExceeded}, WorkerID: "worker-a", Clock: application.ClockFunc(func() time.Time { return now })})
	if _, replayed, err := service.Replay(context.Background(), 8); err == nil || replayed || !store.returnCalled || store.returned.ID != 8 || store.returned.Fence != 4 || store.marked.ID != 0 {
		t.Fatalf("Replay() replayed=%t err=%v marked=%#v returned=%#v", replayed, err, store.marked, store.returned)
	}
}
