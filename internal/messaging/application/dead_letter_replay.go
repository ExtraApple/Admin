package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"admin/internal/messaging/domain"
)

const ConsumerDeadLetterReplayLease = 30 * time.Second

type ConsumerDeadLetterReplayPublisher interface {
	PublishConsumerReplay(context.Context, string, domain.MessageEvent) error
}

type ConsumerDeadLetterReplayConfig struct {
	Store     ConsumerDeadLetterStore
	Publisher ConsumerDeadLetterReplayPublisher
	WorkerID  string
	Clock     Clock
	Logger    RuntimeLogger
}

type ConsumerDeadLetterReplayService struct {
	store     ConsumerDeadLetterStore
	publisher ConsumerDeadLetterReplayPublisher
	workerID  string
	clock     Clock
	logger    RuntimeLogger
}

func NewConsumerDeadLetterReplayService(config ConsumerDeadLetterReplayConfig) *ConsumerDeadLetterReplayService {
	if config.Clock == nil {
		config.Clock = ClockFunc(time.Now)
	}
	if config.Logger == nil {
		config.Logger = discardRuntimeLogger{}
	}
	return &ConsumerDeadLetterReplayService{store: config.Store, publisher: config.Publisher, workerID: config.WorkerID, clock: config.Clock, logger: config.Logger}
}

func (service *ConsumerDeadLetterReplayService) Replay(ctx context.Context, id uint) (ConsumerDeadLetter, bool, error) {
	if service == nil || service.store == nil || service.publisher == nil || service.workerID == "" || id == 0 {
		return ConsumerDeadLetter{}, false, ErrMessagingDependency
	}
	now := service.clock.Now().UTC()
	deadLetter, acquired, err := service.store.ClaimConsumerDeadLetterReplay(ctx, ConsumerDeadLetterReplayClaim{ID: id, WorkerID: service.workerID, Now: now, Lease: ConsumerDeadLetterReplayLease})
	if err != nil {
		service.logger.Warn("messaging_consumer_dlq_replay_claim_failed", runtimeField("stage", "claim"), runtimeField("projection_id", id), runtimeField("failure_code", "consumer_dlq_replay_claim_failed"))
		return deadLetter, false, err
	}
	if !acquired {
		service.logger.Info("messaging_consumer_dlq_replay_lease_unavailable", runtimeField("stage", "claim"), runtimeField("projection_id", id), runtimeField("failure_code", "consumer_dlq_replay_lease_unavailable"))
		return deadLetter, false, nil
	}
	if deadLetter.Invalid || (deadLetter.Fingerprint != "" && !deadLetter.Replayable) {
		result := ConsumerDeadLetterReplayResult{ID: deadLetter.ID, WorkerID: service.workerID, Fence: deadLetter.ReplayLeaseFence, Now: service.clock.Now().UTC()}
		_, _ = service.store.ReturnConsumerDeadLetterPending(ctx, result)
		return deadLetter, false, ErrConsumerDeadLetterInvalid
	}
	result := ConsumerDeadLetterReplayResult{ID: deadLetter.ID, WorkerID: service.workerID, Fence: deadLetter.ReplayLeaseFence, Now: service.clock.Now().UTC()}
	if err := service.publisher.PublishConsumerReplay(ctx, deadLetter.ConsumerName, deadLetter.Event); err != nil {
		service.logger.Warn("messaging_consumer_dlq_replay_publish_failed", runtimeField("stage", "publish"), runtimeField("projection_id", deadLetter.ID), runtimeField("event_id", deadLetter.Event.EventID), runtimeField("failure_code", "consumer_dlq_replay_publish_failed"), runtimeField("retry_attempt", deadLetter.RetryAttempt))
		if _, returnErr := service.store.ReturnConsumerDeadLetterPending(ctx, result); returnErr != nil {
			service.logger.Warn("messaging_consumer_dlq_replay_return_failed", runtimeField("stage", "return_pending"), runtimeField("projection_id", deadLetter.ID), runtimeField("event_id", deadLetter.Event.EventID), runtimeField("failure_code", "consumer_dlq_replay_return_failed"))
			return deadLetter, false, errors.Join(fmt.Errorf("publish Consumer dead-letter replay: %w", err), fmt.Errorf("return Consumer dead letter pending: %w", returnErr))
		}
		return deadLetter, false, fmt.Errorf("publish Consumer dead-letter replay: %w", err)
	}
	if finalized, finalizeErr := service.store.MarkConsumerDeadLetterReplayed(ctx, result); finalizeErr != nil {
		service.logger.Warn("messaging_consumer_dlq_replay_finalize_failed", runtimeField("stage", "finalize"), runtimeField("projection_id", deadLetter.ID), runtimeField("event_id", deadLetter.Event.EventID), runtimeField("failure_code", "consumer_dlq_replay_finalize_failed"))
		return deadLetter, false, finalizeErr
	} else if !finalized {
		service.logger.Warn("messaging_consumer_dlq_replay_lease_lost", runtimeField("stage", "finalize"), runtimeField("projection_id", deadLetter.ID), runtimeField("event_id", deadLetter.Event.EventID), runtimeField("failure_code", "consumer_dlq_replay_lease_lost"))
		return deadLetter, false, ErrLeaseNotHeld
	}
	service.logger.Info("messaging_consumer_dlq_replayed", runtimeField("stage", "finalize"), runtimeField("projection_id", deadLetter.ID), runtimeField("event_id", deadLetter.Event.EventID), runtimeField("result", "replayed"), runtimeField("replay_cycle", deadLetter.ReplayCycle))
	return deadLetter, true, nil
}
