package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type consumerMessageStoreFake struct{ rules []domain.AudienceRule }

func (store consumerMessageStoreFake) PersistMessage(context.Context, application.MessagePersistence) (domain.Message, error) {
	panic("unused")
}
func (store consumerMessageStoreFake) FindMessage(context.Context, uint) (domain.Message, error) {
	return domain.Message{}, nil
}
func (store consumerMessageStoreFake) ListMessages(context.Context, application.MessageListQuery) ([]domain.Message, int64, error) {
	panic("unused")
}
func (store consumerMessageStoreFake) ChangeMessage(context.Context, application.MessageChange) (domain.Message, error) {
	panic("unused")
}
func (store consumerMessageStoreFake) ListAudienceRules(context.Context, uint) ([]domain.AudienceRule, error) {
	return append([]domain.AudienceRule(nil), store.rules...), nil
}

type consumerOrganizationFake struct {
	members map[uint][]uint
	roles   map[[2]uint][]uint
}

func (organization consumerOrganizationFake) Memberships(context.Context, uint) ([]application.OrganizationMembership, error) {
	return nil, nil
}
func (organization consumerOrganizationFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	return nil, nil
}
func (organization consumerOrganizationFake) MemberUserIDs(_ context.Context, organizationID uint) ([]uint, error) {
	return append([]uint(nil), organization.members[organizationID]...), nil
}
func (organization consumerOrganizationFake) RoleMemberUserIDs(_ context.Context, organizationID, roleID uint) ([]uint, error) {
	return append([]uint(nil), organization.roles[[2]uint{organizationID, roleID}]...), nil
}

type consumerIdentityFake struct {
	users map[uint]application.IdentityUser
}

func (identity consumerIdentityFake) LookupUser(_ context.Context, id uint) (application.IdentityUser, error) {
	return identity.users[id], nil
}
func (consumerIdentityFake) LookupVerifiedEmail(context.Context, uint) (application.VerifiedEmail, bool, error) {
	return application.VerifiedEmail{}, false, nil
}

type consumerStoreFake struct {
	claim           application.EventConsumption
	acquired        bool
	batches         []application.AudienceDeliveryBatch
	completion      application.EventConsumptionFinalization
	failed          application.EventConsumptionFailure
	snapshotUserIDs []uint
	resetCalls      int
	marked          int
	renewCalls      int
	failRenewAt     int
}

func (store *consumerStoreFake) ClaimEventConsumption(context.Context, application.EventConsumptionClaim) (application.EventConsumption, bool, error) {
	return store.claim, store.acquired, nil
}
func (store *consumerStoreFake) RenewEventConsumptionLease(_ context.Context, _ application.EventConsumptionLease) (bool, error) {
	store.renewCalls++
	if store.failRenewAt > 0 && store.renewCalls >= store.failRenewAt {
		return false, nil
	}
	return true, nil
}

func (store *consumerStoreFake) ResetIncompleteAudienceSnapshot(context.Context, application.EventConsumptionLease) (bool, error) {
	store.resetCalls++
	store.snapshotUserIDs = nil
	return true, nil
}
func (store *consumerStoreFake) PersistAudienceDeliveryBatch(_ context.Context, batch application.AudienceDeliveryBatch) (bool, error) {
	store.batches = append(store.batches, batch)
	return true, nil
}
func (store *consumerStoreFake) FailEventConsumption(_ context.Context, failure application.EventConsumptionFailure) (bool, error) {
	store.failed = failure
	return true, nil
}

func (store *consumerStoreFake) MarkAudienceSnapshotComplete(context.Context, application.EventConsumptionLease) (bool, error) {
	store.marked++
	store.claim.SnapshotComplete = true
	return true, nil
}
func (store *consumerStoreFake) FinalizeEventConsumption(_ context.Context, _ application.EventConsumptionCompletion) (application.EventConsumptionFinalization, error) {
	store.completion = application.EventConsumptionFinalization{Status: domain.EventConsumptionStatusCompleted, Completed: true}
	return store.completion, nil
}
func (store *consumerStoreFake) ListAudienceDeliveryUserIDs(context.Context, string, string) ([]uint, error) {
	return append([]uint(nil), store.snapshotUserIDs...), nil
}
func (store *consumerStoreFake) CleanupAudienceDeliveries(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

type refreshStreamFake struct{ refreshes []application.RefreshEvent }

func (stream *refreshStreamFake) PublishRefresh(_ context.Context, refresh application.RefreshEvent) error {
	stream.refreshes = append(stream.refreshes, refresh)
	return nil
}
func TestEventConsumerSnapshotsDeduplicatedEnabledAudienceBeforeRefreshWrites(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &consumerStoreFake{claim: application.EventConsumption{ConsumerName: "websocket", EventID: "event-1", MessageCopyID: 41, AggregateVersion: 1, Status: domain.EventConsumptionStatusSnapshotting, WorkerID: "worker-a", SnapshotFence: 3, LeaseExpiresAt: timePtr(now.Add(time.Minute))}, acquired: true}
	stream := &refreshStreamFake{}
	consumer := application.NewEventConsumer(application.EventConsumerConfig{ConsumerName: "websocket", WorkerID: "worker-a", Lease: 30 * time.Second, BatchSize: 500, MaxAudienceUsers: 100000, Messages: consumerMessageStoreFake{rules: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}, {OrganizationID: 10, Type: domain.AudienceTypeRole, RoleID: 3}}}, Organizations: consumerOrganizationFake{members: map[uint][]uint{10: {2, 3}}, roles: map[[2]uint][]uint{{10, 3}: {3, 4}}}, Identity: consumerIdentityFake{users: map[uint]application.IdentityUser{2: {ID: 2, Enabled: true}, 3: {ID: 3, Enabled: true}, 4: {ID: 4, Enabled: false}}}, Store: store, Stream: stream, Clock: application.ClockFunc(func() time.Time { return now })})
	event := domain.MessageEvent{EventID: "event-1", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	if result, err := consumer.Process(context.Background(), event); err != nil || !result.Completed || len(store.batches) != 1 || len(store.batches[0].UserIDs) != 2 || len(stream.refreshes) != 2 || store.completion.Status != domain.EventConsumptionStatusCompleted {
		t.Fatalf("Process() result=%#v err=%v batches=%#v refreshes=%#v completion=%#v", result, err, store.batches, stream.refreshes, store.completion)
	}
	if stream.refreshes[0].EventID != event.EventID || stream.refreshes[0].MessageCopyID != event.MessageCopyID || stream.refreshes[0].UserID != 2 {
		t.Fatalf("refreshes = %#v", stream.refreshes)
	}
}

func TestEventConsumerRejectsAudienceOverflowWithoutSnapshotOrRefresh(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &consumerStoreFake{claim: application.EventConsumption{ConsumerName: "websocket", EventID: "event-overflow", MessageCopyID: 41, AggregateVersion: 1, Status: domain.EventConsumptionStatusSnapshotting, WorkerID: "worker-a", SnapshotFence: 3, LeaseExpiresAt: timePtr(now.Add(time.Minute))}, acquired: true}
	members := make([]uint, 100001)
	users := make(map[uint]application.IdentityUser, len(members))
	for index := range members {
		members[index] = uint(index + 1)
		users[members[index]] = application.IdentityUser{ID: members[index], Enabled: true}
	}
	consumer := application.NewEventConsumer(application.EventConsumerConfig{ConsumerName: "websocket", WorkerID: "worker-a", Lease: 30 * time.Second, BatchSize: 500, MaxAudienceUsers: 100000, Messages: consumerMessageStoreFake{rules: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}}, Organizations: consumerOrganizationFake{members: map[uint][]uint{10: members}}, Identity: consumerIdentityFake{users: users}, Store: store, Stream: &refreshStreamFake{}, Clock: application.ClockFunc(func() time.Time { return now })})
	event := domain.MessageEvent{EventID: "event-overflow", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	if result, err := consumer.Process(context.Background(), event); !errors.Is(err, domain.ErrAudienceCapacityExceeded) || result.Completed || result.AudienceObservedCount != 100001 || store.failed.FailureCode != application.FailureAudienceCapacityExceeded || store.failed.AudienceObservedCount != 100001 || len(store.batches) != 0 {
		t.Fatalf("overflow Process() result=%#v err=%v failed=%#v batches=%#v", result, err, store.failed, store.batches)
	}
}

func TestEventConsumerRecalculatesAudienceAfterCapacityFailure(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	members := map[uint][]uint{10: {2, 3}}
	store := &consumerStoreFake{claim: application.EventConsumption{ConsumerName: "websocket", EventID: "event-capacity-retry", MessageCopyID: 41, AggregateVersion: 1, Status: domain.EventConsumptionStatusSnapshotting, WorkerID: "worker-a", SnapshotFence: 3, LeaseExpiresAt: timePtr(now.Add(time.Minute))}, acquired: true}
	stream := &refreshStreamFake{}
	consumer := application.NewEventConsumer(application.EventConsumerConfig{ConsumerName: "websocket", WorkerID: "worker-a", Lease: 30 * time.Second, BatchSize: 500, MaxAudienceUsers: 1, Messages: consumerMessageStoreFake{rules: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}}, Organizations: consumerOrganizationFake{members: members}, Identity: consumerIdentityFake{users: map[uint]application.IdentityUser{2: {ID: 2, Enabled: true}, 3: {ID: 3, Enabled: true}}}, Store: store, Stream: stream, Clock: application.ClockFunc(func() time.Time { return now })})
	event := domain.MessageEvent{EventID: "event-capacity-retry", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	if _, err := consumer.Process(context.Background(), event); !errors.Is(err, domain.ErrAudienceCapacityExceeded) {
		t.Fatalf("first Process() error = %v, want capacity failure", err)
	}
	members[10] = []uint{2}
	if result, err := consumer.Process(context.Background(), event); err != nil || !result.Completed || result.AudienceObservedCount != 1 || len(store.batches) != 1 || len(store.batches[0].UserIDs) != 1 || store.batches[0].UserIDs[0] != 2 || len(stream.refreshes) != 1 || stream.refreshes[0].UserID != 2 {
		t.Fatalf("retry Process() result=%#v err=%v batches=%#v refreshes=%#v", result, err, store.batches, stream.refreshes)
	}
}

func TestEventConsumerStopsAfterSnapshotLeaseLossBeforeRefreshAndAck(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &consumerStoreFake{claim: application.EventConsumption{ConsumerName: "websocket", EventID: "event-fence", MessageCopyID: 41, AggregateVersion: 1, Status: domain.EventConsumptionStatusSnapshotting, WorkerID: "worker-a", SnapshotFence: 3, LeaseExpiresAt: timePtr(now.Add(time.Minute))}, acquired: true, failRenewAt: 2}
	stream := &refreshStreamFake{}
	consumer := application.NewEventConsumer(application.EventConsumerConfig{ConsumerName: "websocket", WorkerID: "worker-a", Lease: 30 * time.Second, BatchSize: 500, MaxAudienceUsers: 100000, Messages: consumerMessageStoreFake{rules: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}}, Organizations: consumerOrganizationFake{members: map[uint][]uint{10: {2}}, roles: map[[2]uint][]uint{}}, Identity: consumerIdentityFake{users: map[uint]application.IdentityUser{2: {ID: 2, Enabled: true}}}, Store: store, Stream: stream, Clock: application.ClockFunc(func() time.Time { return now })})
	event := domain.MessageEvent{EventID: "event-fence", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	if _, err := consumer.Process(context.Background(), event); err != application.ErrLeaseNotHeld {
		t.Fatalf("Process() error = %v, want ErrLeaseNotHeld", err)
	}
	if len(store.batches) != 1 || len(stream.refreshes) != 0 || store.completion.Completed {
		t.Fatalf("lease-loss state batches=%#v refreshes=%#v completion=%#v", store.batches, stream.refreshes, store.completion)
	}
}

func timePtr(value time.Time) *time.Time { return &value }

func TestEventConsumerReusesCompletedSnapshotInsteadOfResolvingCurrentAudience(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &consumerStoreFake{claim: application.EventConsumption{ConsumerName: "websocket", EventID: "event-retry", MessageCopyID: 41, AggregateVersion: 1, Status: domain.EventConsumptionStatusSnapshotting, WorkerID: "worker-a", SnapshotFence: 4, SnapshotComplete: true, LeaseExpiresAt: timePtr(now.Add(time.Minute))}, acquired: true, snapshotUserIDs: []uint{2}}
	stream := &refreshStreamFake{}
	consumer := application.NewEventConsumer(application.EventConsumerConfig{ConsumerName: "websocket", WorkerID: "worker-a", Lease: 30 * time.Second, BatchSize: 500, MaxAudienceUsers: 100000, Messages: consumerMessageStoreFake{rules: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}}, Organizations: consumerOrganizationFake{members: map[uint][]uint{10: {3}}}, Identity: consumerIdentityFake{users: map[uint]application.IdentityUser{2: {ID: 2, Enabled: true}}}, Store: store, Stream: stream, Clock: application.ClockFunc(func() time.Time { return now })})
	event := domain.MessageEvent{EventID: "event-retry", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}
	if result, err := consumer.Process(context.Background(), event); err != nil || !result.Completed || len(store.batches) != 0 || len(stream.refreshes) != 1 || stream.refreshes[0].UserID != 2 {
		t.Fatalf("Process() result=%#v err=%v batches=%#v refreshes=%#v", result, err, store.batches, stream.refreshes)
	}
}
