package domain

import (
	"errors"
	"time"
)

type EventName string

const (
	EventNameMessageCreated   EventName = "messaging.message.created.v1"
	EventNameMessagePublished EventName = "messaging.message.published.v1"
	EventNameMessageEdited    EventName = "messaging.message.edited.v1"
	EventNameMessageRevoked   EventName = "messaging.message.revoked.v1"
	EventNameMessageExpired   EventName = "messaging.message.expired.v1"
)

type MessageEvent struct {
	EventID          string
	EventName        EventName
	EventVersion     uint
	MessageCopyID    uint
	OrganizationID   uint
	OccurredAt       time.Time
	AggregateVersion uint64
}

type OutboxStatus string

const (
	OutboxStatusPending    OutboxStatus = "pending"
	OutboxStatusPublishing OutboxStatus = "publishing"
	OutboxStatusPublished  OutboxStatus = "published"
	OutboxStatusDead       OutboxStatus = "dead"
)

type MessageOutbox struct {
	ID              uint
	Event           MessageEvent
	Status          OutboxStatus
	RetryAttempt    int
	NextRetryAt     *time.Time
	WorkerID        string
	LeaseExpiresAt  *time.Time
	LastFailureCode string
}

type EventConsumptionStatus string

const (
	EventConsumptionStatusSnapshotting EventConsumptionStatus = "snapshotting"
	EventConsumptionStatusCompleted    EventConsumptionStatus = "completed"
	EventConsumptionStatusFailed       EventConsumptionStatus = "failed"
	EventConsumptionStatusSuperseded   EventConsumptionStatus = "superseded"
)

type EventConsumption struct {
	ConsumerName          string
	EventID               string
	MessageCopyID         uint
	Status                EventConsumptionStatus
	WorkerID              string
	SnapshotFence         uint64
	LeaseExpiresAt        *time.Time
	FailureCode           string
	AudienceObservedCount int
}

type AudienceSnapshot struct {
	EventID       string
	Users         []uint
	ObservedCount int
}

type ConsumerDLQStatus string

const (
	ConsumerDLQStatusPending   ConsumerDLQStatus = "pending"
	ConsumerDLQStatusReplaying ConsumerDLQStatus = "replaying"
	ConsumerDLQStatusReplayed  ConsumerDLQStatus = "replayed"
	ConsumerDLQStatusDiscarded ConsumerDLQStatus = "discarded"
)

type ConsumerDeadLetter struct {
	ConsumerName          string
	EventID               string
	Status                ConsumerDLQStatus
	ReplayCycle           uint
	ReplayLeaseFence      uint64
	ReplayLeaseOwner      string
	ReplayLeaseExpiresAt  *time.Time
	LastFailureCode       string
	AudienceObservedCount int
}

var (
	ErrMessageEventInvalid      = errors.New("message event is invalid")
	ErrSnapshotLeaseHeld        = errors.New("message event snapshot lease is held")
	ErrSnapshotLeaseOwner       = errors.New("message event snapshot lease owner is invalid")
	ErrSnapshotLeaseInvalid     = errors.New("message event snapshot lease is invalid")
	ErrSnapshotAlreadyCompleted = errors.New("message event snapshot is already completed")
	ErrAudienceCapacityExceeded = errors.New("audience_capacity_exceeded")
	ErrConsumerDLQInvalid       = errors.New("consumer dead letter is invalid")
)

func ValidateMessageEvent(event MessageEvent) error {
	if event.EventID == "" || !validEventName(event.EventName) || event.EventVersion == 0 || event.MessageCopyID == 0 || event.OrganizationID == 0 || event.OccurredAt.IsZero() || event.AggregateVersion == 0 {
		return ErrMessageEventInvalid
	}
	return nil
}

func ClaimSnapshotLease(consumption *EventConsumption, workerID string, now time.Time, lease time.Duration) error {
	if consumption == nil || consumption.ConsumerName == "" || consumption.EventID == "" || workerID == "" || lease <= 0 {
		return ErrSnapshotLeaseInvalid
	}
	if consumption.Status == EventConsumptionStatusCompleted || consumption.Status == EventConsumptionStatusSuperseded {
		return ErrSnapshotAlreadyCompleted
	}
	if consumption.LeaseExpiresAt != nil && now.Before(*consumption.LeaseExpiresAt) && consumption.WorkerID != workerID {
		return ErrSnapshotLeaseHeld
	}
	consumption.SnapshotFence++
	consumption.Status = EventConsumptionStatusSnapshotting
	consumption.WorkerID = workerID
	expiresAt := now.Add(lease)
	consumption.LeaseExpiresAt = &expiresAt
	return nil
}

func RenewSnapshotLease(consumption *EventConsumption, workerID string, fence uint64, now time.Time, lease time.Duration) error {
	if !snapshotLeaseOwned(consumption, workerID, fence, now) || lease <= 0 {
		return ErrSnapshotLeaseOwner
	}
	expiresAt := now.Add(lease)
	consumption.LeaseExpiresAt = &expiresAt
	return nil
}

func CompleteSnapshot(consumption *EventConsumption, workerID string, fence uint64, now time.Time) error {
	if !snapshotLeaseOwned(consumption, workerID, fence, now) {
		return ErrSnapshotLeaseOwner
	}
	consumption.Status = EventConsumptionStatusCompleted
	consumption.WorkerID = ""
	consumption.LeaseExpiresAt = nil
	return nil
}

func NewAudienceSnapshot(eventID string, userIDs []uint, maxUsers int) (AudienceSnapshot, error) {
	snapshot := AudienceSnapshot{EventID: eventID, ObservedCount: len(userIDs)}
	if eventID == "" || maxUsers < 1 || len(userIDs) > maxUsers {
		return snapshot, ErrAudienceCapacityExceeded
	}
	snapshot.Users = append([]uint(nil), userIDs...)
	return snapshot, nil
}

func BeginConsumerDLQReplay(deadLetter *ConsumerDeadLetter, workerID string, now time.Time, lease time.Duration) error {
	if deadLetter == nil || deadLetter.ConsumerName == "" || deadLetter.EventID == "" || workerID == "" || lease <= 0 || deadLetter.Status != ConsumerDLQStatusPending {
		return ErrConsumerDLQInvalid
	}
	deadLetter.Status = ConsumerDLQStatusReplaying
	deadLetter.ReplayCycle++
	deadLetter.ReplayLeaseFence++
	deadLetter.ReplayLeaseOwner = workerID
	expiresAt := now.Add(lease)
	deadLetter.ReplayLeaseExpiresAt = &expiresAt
	return nil
}

func MarkConsumerDLQReplayed(deadLetter *ConsumerDeadLetter, workerID string, fence uint64) error {
	if deadLetter == nil || deadLetter.Status != ConsumerDLQStatusReplaying || deadLetter.ReplayLeaseOwner != workerID || deadLetter.ReplayLeaseFence != fence {
		return ErrConsumerDLQInvalid
	}
	deadLetter.Status = ConsumerDLQStatusReplayed
	deadLetter.ReplayLeaseOwner = ""
	deadLetter.ReplayLeaseExpiresAt = nil
	return nil
}

func ReopenConsumerDLQAfterFailure(deadLetter *ConsumerDeadLetter, failureCode string) error {
	if deadLetter == nil || deadLetter.Status != ConsumerDLQStatusReplayed || failureCode == "" {
		return ErrConsumerDLQInvalid
	}
	deadLetter.Status = ConsumerDLQStatusPending
	deadLetter.ReplayCycle++
	deadLetter.LastFailureCode = failureCode
	deadLetter.ReplayLeaseOwner = ""
	deadLetter.ReplayLeaseExpiresAt = nil
	return nil
}

func snapshotLeaseOwned(consumption *EventConsumption, workerID string, fence uint64, now time.Time) bool {
	return consumption != nil && consumption.Status == EventConsumptionStatusSnapshotting && consumption.WorkerID == workerID && consumption.SnapshotFence == fence && consumption.LeaseExpiresAt != nil && now.Before(*consumption.LeaseExpiresAt)
}

func validEventName(name EventName) bool {
	switch name {
	case EventNameMessageCreated, EventNameMessagePublished, EventNameMessageEdited, EventNameMessageRevoked, EventNameMessageExpired:
		return true
	default:
		return false
	}
}
