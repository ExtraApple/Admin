package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"admin/internal/messaging/domain"
)

const (
	FailureAudienceCapacityExceeded = "audience_capacity_exceeded"
	DefaultConsumerBatchSize        = 500
	DefaultConsumerMaxAudienceUsers = 100000
	ConsumerSnapshotRetention       = 24 * time.Hour
)

type RefreshEvent struct {
	EventID          string
	Cursor           string
	MessageCopyID    uint
	UserID           uint
	AggregateVersion uint64
}

type RefreshStream interface {
	PublishRefreshBatch(context.Context, []RefreshEvent) error
}

// SnapshotTransactionRunner runs audience resolution and snapshot persistence
// in one database transaction with REPEATABLE READ isolation.
type SnapshotTransactionRunner interface {
	RunRepeatableRead(context.Context, func(context.Context) error) error
}

type directSnapshotTransactionRunner struct{}

func (directSnapshotTransactionRunner) RunRepeatableRead(ctx context.Context, operation func(context.Context) error) error {
	return operation(ctx)
}

type RefreshRecovery struct {
	Events      []RefreshEvent
	FullRefresh bool
}

// RefreshRecoveryStore provides bounded replay of a user's durable refresh hints.
// A store requests FullRefresh rather than fabricating history if its retention
// window no longer contains every event after the supplied cursor.
type RefreshRecoveryStore interface {
	ReplayRefresh(context.Context, uint, string) (RefreshRecovery, error)
}

type EventConsumerConfig struct {
	ConsumerName         string
	WorkerID             string
	Lease                time.Duration
	BatchSize            int
	MaxAudienceUsers     int
	Messages             MessageStore
	Organizations        OrganizationAudienceReader
	Identity             IdentityReader
	Store                ConsumerStore
	Stream               RefreshStream
	SnapshotTransactions SnapshotTransactionRunner
	Clock                Clock
	Logger               RuntimeLogger
}

type EventConsumer struct {
	consumerName         string
	workerID             string
	lease                time.Duration
	batchSize            int
	maxAudienceUsers     int
	messages             MessageStore
	organizations        OrganizationAudienceReader
	identity             IdentityReader
	store                ConsumerStore
	stream               RefreshStream
	snapshotTransactions SnapshotTransactionRunner
	clock                Clock
	logger               RuntimeLogger
}

type EventConsumerResult struct {
	Completed             bool
	Superseded            bool
	AudienceObservedCount int
}

func NewEventConsumer(config EventConsumerConfig) *EventConsumer {
	if config.Clock == nil {
		config.Clock = ClockFunc(time.Now)
	}
	if config.Lease <= 0 {
		config.Lease = 30 * time.Second
	}
	if config.BatchSize <= 0 || config.BatchSize > DefaultConsumerBatchSize {
		config.BatchSize = DefaultConsumerBatchSize
	}
	if config.MaxAudienceUsers <= 0 {
		config.MaxAudienceUsers = DefaultConsumerMaxAudienceUsers
	}
	if config.SnapshotTransactions == nil {
		config.SnapshotTransactions = directSnapshotTransactionRunner{}
	}
	if config.Logger == nil {
		config.Logger = discardRuntimeLogger{}
	}
	return &EventConsumer{consumerName: config.ConsumerName, workerID: config.WorkerID, lease: config.Lease, batchSize: config.BatchSize, maxAudienceUsers: config.MaxAudienceUsers, messages: config.Messages, organizations: config.Organizations, identity: config.Identity, store: config.Store, stream: config.Stream, snapshotTransactions: config.SnapshotTransactions, clock: config.Clock, logger: config.Logger}
}
func (consumer *EventConsumer) logFailure(event domain.MessageEvent, stage, failureCode string, retryAttempt int, fields ...RuntimeLogField) {
	base := []RuntimeLogField{runtimeField("consumer", consumer.consumerName), runtimeField("event_id", event.EventID), runtimeField("stage", stage), runtimeField("failure_code", failureCode), runtimeField("retry_attempt", retryAttempt)}
	consumer.logger.Warn("messaging_consumer_failed", append(base, fields...)...)
}

func (consumer *EventConsumer) discardIncompleteAudienceSnapshot(ctx context.Context, lease EventConsumptionLease, event domain.MessageEvent, cause error) error {
	discarded, err := consumer.store.DiscardAudienceDeliverySnapshot(ctx, lease)
	if err != nil {
		consumer.logFailure(event, "snapshot_discard", "consumer_snapshot_discard_failed", 0)
		return err
	}
	if !discarded {
		consumer.logFailure(event, "snapshot_discard", "consumer_snapshot_fence_lost", 0)
		return ErrLeaseNotHeld
	}
	return cause
}

func eventConsumerFailureCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrAudienceCapacityExceeded):
		return FailureAudienceCapacityExceeded
	case errors.Is(err, ErrLeaseNotHeld):
		return "consumer_snapshot_fence_lost"
	default:
		return "consumer_processing_failed"
	}
}

func (consumer *EventConsumer) Process(ctx context.Context, event domain.MessageEvent) (EventConsumerResult, error) {
	if consumer == nil || consumer.consumerName == "" || consumer.workerID == "" || consumer.messages == nil || consumer.organizations == nil || consumer.identity == nil || consumer.store == nil || consumer.stream == nil {
		return EventConsumerResult{}, ErrMessagingDependency
	}
	if err := domain.ValidateMessageEvent(event); err != nil {
		consumer.logFailure(event, "validate", "message_event_invalid", 0)
		return EventConsumerResult{}, err
	}
	now := consumer.clock.Now().UTC()
	claim, acquired, err := consumer.store.ClaimEventConsumption(ctx, EventConsumptionClaim{ConsumerName: consumer.consumerName, Event: event, WorkerID: consumer.workerID, Now: now, Lease: consumer.lease})
	if err != nil {
		consumer.logFailure(event, "claim", "consumer_claim_failed", 0)
		return EventConsumerResult{}, err
	}
	if !acquired {
		result := EventConsumerResult{Completed: claim.Status == domain.EventConsumptionStatusCompleted || claim.Status == domain.EventConsumptionStatusSuperseded, Superseded: claim.Status == domain.EventConsumptionStatusSuperseded, AudienceObservedCount: claim.AudienceObservedCount}
		consumer.logger.Info("messaging_consumer_duplicate_or_superseded", runtimeField("consumer", consumer.consumerName), runtimeField("event_id", event.EventID), runtimeField("state", string(claim.Status)))
		return result, nil
	}
	lease := EventConsumptionLease{ConsumerName: consumer.consumerName, EventID: event.EventID, WorkerID: consumer.workerID, SnapshotFence: claim.SnapshotFence, Now: now, Lease: consumer.lease}
	var userIDs []uint
	if claim.SnapshotComplete {
		userIDs, err = consumer.store.ListAudienceDeliveryUserIDs(ctx, consumer.consumerName, event.EventID)
		if err != nil {
			consumer.logFailure(event, "snapshot_read", "consumer_snapshot_read_failed", 0)
			return EventConsumerResult{}, err
		}
	} else {
		lease.Now = consumer.clock.Now().UTC()
		if reset, resetErr := consumer.store.ResetIncompleteAudienceSnapshot(ctx, lease); resetErr != nil {
			consumer.logFailure(event, "snapshot_reset", eventConsumerFailureCode(resetErr), 0)
			return EventConsumerResult{}, resetErr
		} else if !reset {
			consumer.logFailure(event, "snapshot_reset", "consumer_snapshot_fence_lost", 0)
			return EventConsumerResult{}, ErrLeaseNotHeld
		}

		snapshotErr := consumer.snapshotTransactions.RunRepeatableRead(ctx, func(snapshotCtx context.Context) error {
			var resolveErr error
			userIDs, resolveErr = consumer.resolveAudience(snapshotCtx, event.MessageCopyID)
			if resolveErr != nil {
				consumer.logFailure(event, "audience_resolve", eventConsumerFailureCode(resolveErr), 0)
				return resolveErr
			}
			if len(userIDs) > consumer.maxAudienceUsers {
				consumer.logFailure(event, "audience_snapshot", FailureAudienceCapacityExceeded, 0, runtimeField("audience_observed_count", len(userIDs)))
				return domain.ErrAudienceCapacityExceeded
			}
			for start := 0; start < len(userIDs); start += consumer.batchSize {
				end := start + consumer.batchSize
				if end > len(userIDs) {
					end = len(userIDs)
				}
				batch := append([]uint(nil), userIDs[start:end]...)
				lease.Now = consumer.clock.Now().UTC()
				if renewed, renewErr := consumer.store.RenewEventConsumptionLease(ctx, lease); renewErr != nil {
					consumer.logFailure(event, "snapshot_renew", "consumer_snapshot_fence_lost", 0)
					return renewErr
				} else if !renewed {
					consumer.logFailure(event, "snapshot_renew", "consumer_snapshot_fence_lost", 0)
					return ErrLeaseNotHeld
				}
				if ok, persistErr := consumer.store.PersistAudienceDeliveryBatch(snapshotCtx, AudienceDeliveryBatch{EventConsumptionLease: lease, MessageCopyID: event.MessageCopyID, UserIDs: batch, ExpiresAt: lease.Now.Add(ConsumerSnapshotRetention)}); persistErr != nil {
					consumer.logFailure(event, "snapshot_persist", "consumer_snapshot_persist_failed", 0)
					return persistErr
				} else if !ok {
					consumer.logFailure(event, "snapshot_persist", "consumer_snapshot_fence_lost", 0)
					return ErrLeaseNotHeld
				}
			}
			lease.Now = consumer.clock.Now().UTC()
			if completed, completeErr := consumer.store.MarkAudienceSnapshotComplete(snapshotCtx, lease); completeErr != nil {
				consumer.logFailure(event, "snapshot_complete", "consumer_snapshot_complete_failed", 0)
				return completeErr
			} else if !completed {
				consumer.logFailure(event, "snapshot_complete", "consumer_snapshot_fence_lost", 0)
				return ErrLeaseNotHeld
			}
			return nil
		})
		if errors.Is(snapshotErr, domain.ErrAudienceCapacityExceeded) {
			consumer.logFailure(event, "audience_snapshot", FailureAudienceCapacityExceeded, 0, runtimeField("audience_observed_count", len(userIDs)))
			if failed, failErr := consumer.store.FailEventConsumption(ctx, EventConsumptionFailure{EventConsumptionLease: lease, FailureCode: FailureAudienceCapacityExceeded, AudienceObservedCount: len(userIDs)}); failErr != nil {
				consumer.logFailure(event, "failure_record", "consumer_failure_record_failed", 0)
				return EventConsumerResult{}, failErr
			} else if !failed {
				consumer.logFailure(event, "failure_record", "consumer_snapshot_fence_lost", 0)
				return EventConsumerResult{}, ErrLeaseNotHeld
			}
			return EventConsumerResult{AudienceObservedCount: len(userIDs)}, domain.ErrAudienceCapacityExceeded
		}
		if snapshotErr != nil {
			return EventConsumerResult{}, consumer.discardIncompleteAudienceSnapshot(ctx, lease, event, snapshotErr)
		}
	}
	observed := len(userIDs)
	lease.Now = consumer.clock.Now().UTC()
	if renewed, renewErr := consumer.store.RenewEventConsumptionLease(ctx, lease); renewErr != nil {
		consumer.logFailure(event, "refresh_renew", "consumer_snapshot_fence_lost", 0)
		return EventConsumerResult{}, renewErr
	} else if !renewed {
		consumer.logFailure(event, "refresh_renew", "consumer_snapshot_fence_lost", 0)
		return EventConsumerResult{}, ErrLeaseNotHeld
	}
	if len(userIDs) > 0 {
		refreshes := make([]RefreshEvent, len(userIDs))
		for index, userID := range userIDs {
			refreshes[index] = RefreshEvent{EventID: event.EventID, Cursor: event.EventID, MessageCopyID: event.MessageCopyID, UserID: userID, AggregateVersion: event.AggregateVersion}
		}
		if err := consumer.stream.PublishRefreshBatch(ctx, refreshes); err != nil {
			consumer.logFailure(event, "refresh_publish", "consumer_refresh_publish_failed", 0)
			return EventConsumerResult{}, err
		}
	}
	lease.Now = consumer.clock.Now().UTC()
	finalization, err := consumer.store.FinalizeEventConsumption(ctx, EventConsumptionCompletion{EventConsumptionLease: lease, MessageCopyID: event.MessageCopyID, AggregateVersion: event.AggregateVersion})
	if err != nil {
		consumer.logFailure(event, "finalize", "consumer_finalize_failed", 0)
		return EventConsumerResult{}, err
	}
	consumer.logger.Info("messaging_consumer_completed", runtimeField("consumer", consumer.consumerName), runtimeField("event_id", event.EventID), runtimeField("state", string(finalization.Status)), runtimeField("audience_observed_count", observed))
	return EventConsumerResult{Completed: finalization.Completed, Superseded: finalization.Status == domain.EventConsumptionStatusSuperseded, AudienceObservedCount: observed}, nil
}

func (consumer *EventConsumer) resolveAudience(ctx context.Context, messageCopyID uint) ([]uint, error) {
	rules, err := consumer.messages.ListAudienceRules(ctx, messageCopyID)
	if err != nil {
		return nil, err
	}
	ids := make(map[uint]struct{})
	for _, rule := range rules {
		var users []uint
		switch rule.Type {
		case domain.AudienceTypeOrganization, domain.AudienceTypeAll:
			users, err = consumer.organizations.MemberUserIDs(ctx, rule.OrganizationID)
		case domain.AudienceTypeRole:
			users, err = consumer.organizations.RoleMemberUserIDs(ctx, rule.OrganizationID, rule.RoleID)
		default:
			return nil, fmt.Errorf("unsupported audience type %q", rule.Type)
		}
		if err != nil {
			return nil, err
		}
		for _, userID := range users {
			if userID == 0 {
				continue
			}
			user, lookupErr := consumer.identity.LookupUser(ctx, userID)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if user.ID == userID && user.Enabled {
				ids[userID] = struct{}{}
			}
		}
	}
	result := make([]uint, 0, len(ids))
	for userID := range ids {
		result = append(result, userID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, nil
}
