package application

import (
	"context"
	"errors"
	"time"

	"admin/internal/messaging/domain"
)

const (
	OutboxFailureCodePublishFailed  = "rabbitmq_publish_failed"
	OutboxFailureCodeConfirmTimeout = "rabbitmq_confirm_timeout"
)

// OutboxPublisher accepts only the minimal event reference. Its implementation
// owns RabbitMQ channels and publisher confirms; Messaging application code
// never receives a broker client.
type OutboxPublisher interface {
	Publish(context.Context, domain.MessageEvent) error
}

type OutboxWorkerConfig struct {
	Store          OutboxStore
	Publisher      OutboxPublisher
	WorkerID       string
	Lease          time.Duration
	ConfirmTimeout time.Duration
	RetryDelays    []time.Duration
	Clock          Clock
	Logger         RuntimeLogger
}

type OutboxWorker struct {
	store          OutboxStore
	publisher      OutboxPublisher
	workerID       string
	lease          time.Duration
	confirmTimeout time.Duration
	retryDelays    []time.Duration
	clock          Clock
	logger         RuntimeLogger
}

func NewOutboxWorker(config OutboxWorkerConfig) *OutboxWorker {
	if config.Clock == nil {
		config.Clock = ClockFunc(time.Now)
	}
	if config.Lease <= 0 {
		config.Lease = 30 * time.Second
	}
	if config.ConfirmTimeout <= 0 {
		config.ConfirmTimeout = 10 * time.Second
	}
	if len(config.RetryDelays) == 0 {
		config.RetryDelays = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	}
	if config.Logger == nil {
		config.Logger = discardRuntimeLogger{}
	}
	return &OutboxWorker{store: config.Store, publisher: config.Publisher, workerID: config.WorkerID, lease: config.Lease, confirmTimeout: config.ConfirmTimeout, retryDelays: append([]time.Duration(nil), config.RetryDelays...), clock: config.Clock, logger: config.Logger}
}

func (worker *OutboxWorker) RunOnce(ctx context.Context, limit int) (int, error) {
	if worker == nil || worker.store == nil || worker.publisher == nil || worker.workerID == "" || worker.lease <= 0 || worker.confirmTimeout <= 0 || len(worker.retryDelays) != 5 || limit < 1 {
		return 0, ErrMessagingDependency
	}
	now := worker.clock.Now().UTC()
	outboxes, err := worker.store.ClaimOutbox(ctx, OutboxClaim{WorkerID: worker.workerID, Now: now, Lease: worker.lease, Limit: limit})
	if err != nil {
		worker.logger.Warn("messaging_outbox_claim_failed", runtimeField("stage", "claim"), runtimeField("worker_id", worker.workerID))
		return 0, err
	}
	published := 0
	for _, outbox := range outboxes {
		lease := OutboxLease{ID: outbox.ID, WorkerID: worker.workerID, Now: worker.clock.Now().UTC(), Lease: worker.lease}
		renewed, err := worker.store.RenewOutboxLease(ctx, lease)
		if err != nil {
			worker.logger.Warn("messaging_outbox_lease_renew_failed", runtimeField("stage", "lease_renew"), runtimeField("outbox_id", outbox.ID), runtimeField("event_id", outbox.Event.EventID))
			return published, err
		}
		if !renewed {
			worker.logger.Info("messaging_outbox_lease_lost", runtimeField("stage", "lease_renew"), runtimeField("outbox_id", outbox.ID), runtimeField("event_id", outbox.Event.EventID))
			continue
		}
		publishContext, cancel := context.WithTimeout(ctx, worker.confirmTimeout)
		err = worker.publisher.Publish(publishContext, outbox.Event)
		cancel()
		if err != nil {
			worker.logger.Warn("messaging_outbox_publish_failed", runtimeField("stage", "publish"), runtimeField("outbox_id", outbox.ID), runtimeField("event_id", outbox.Event.EventID), runtimeField("failure_code", outboxFailureCode(err)), runtimeField("retry_attempt", outbox.RetryAttempt))
			if recordErr := worker.recordFailure(ctx, lease, outbox.RetryAttempt, err); recordErr != nil {
				return published, recordErr
			}
			continue
		}
		lease.Now = worker.clock.Now().UTC()
		marked, err := worker.store.MarkOutboxPublished(ctx, lease)
		if err != nil {
			worker.logger.Warn("messaging_outbox_mark_published_failed", runtimeField("stage", "mark_published"), runtimeField("outbox_id", outbox.ID), runtimeField("event_id", outbox.Event.EventID))
			return published, err
		}
		if marked {
			published++
			worker.logger.Info("messaging_outbox_published", runtimeField("stage", "published"), runtimeField("outbox_id", outbox.ID), runtimeField("event_id", outbox.Event.EventID))
		}
	}
	return published, nil
}

func (worker *OutboxWorker) recordFailure(ctx context.Context, lease OutboxLease, retryAttempt int, publishError error) error {
	attempt := retryAttempt + 1
	failure := OutboxFailure{OutboxLease: lease, FailureCode: outboxFailureCode(publishError), Dead: attempt >= len(worker.retryDelays)}
	if !failure.Dead {
		retryAt := worker.clock.Now().UTC().Add(worker.retryDelays[retryAttempt])
		failure.RetryAt = &retryAt
	}
	_, err := worker.store.RecordOutboxFailure(ctx, failure)
	if err != nil {
		worker.logger.Warn("messaging_outbox_failure_record_failed", runtimeField("stage", "record_failure"), runtimeField("outbox_id", lease.ID), runtimeField("retry_attempt", retryAttempt), runtimeField("dead_lettered", failure.Dead))
		return err
	}
	worker.logger.Info("messaging_outbox_failure_recorded", runtimeField("stage", "record_failure"), runtimeField("outbox_id", lease.ID), runtimeField("retry_attempt", retryAttempt), runtimeField("dead_lettered", failure.Dead))
	return nil
}

func outboxFailureCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return OutboxFailureCodeConfirmTimeout
	}
	return OutboxFailureCodePublishFailed
}
