package application

import (
	"context"
	"errors"
	"time"

	"admin/internal/messaging/domain"
)

var (
	ErrNotFound                  = errors.New("messaging record not found")
	ErrStateConflict             = errors.New("messaging record state conflict")
	ErrLeaseNotHeld              = errors.New("messaging lease is not held")
	ErrAudienceBatchTooLarge     = errors.New("messaging audience delivery batch exceeds 500 users")
	ErrConsumerDeadLetterInvalid = errors.New("messaging consumer dead letter is invalid")
)

// Repository is Messaging's persistence seam. Application modules depend on
// the narrow facet they need; only composition requires the complete store.
type Repository interface {
	CategoryStore
	MessageStore
	InboxStore
	OutboxStore
	ConsumerStore
	ConsumerDeadLetterStore
}

type CategoryStore interface {
	CreateCategory(context.Context, domain.MessageCategory) (domain.MessageCategory, error)
	FindCategory(context.Context, uint) (domain.MessageCategory, error)
	FindCategoryByCode(context.Context, uint, string) (domain.MessageCategory, error)
	ListCategories(context.Context, CategoryListQuery) ([]domain.MessageCategory, int64, error)
	UpdateCategory(context.Context, domain.MessageCategory) error
	DeleteCategoryIfUnused(context.Context, uint) (bool, error)
}

type CategoryListQuery struct {
	OrganizationIDs []uint
	EnabledOnly     bool
	Offset, Limit   int
}

type MessageStore interface {
	PersistMessage(context.Context, MessagePersistence) (domain.Message, error)
	FindMessage(context.Context, uint) (domain.Message, error)
	ListMessages(context.Context, MessageListQuery) ([]domain.Message, int64, error)
	ChangeMessage(context.Context, MessageChange) (domain.Message, error)
	ListAudienceRules(context.Context, uint) ([]domain.AudienceRule, error)
}

// MessagePersistence creates one message copy and all relations that must be
// committed with it. Event is optional only for an unpublished announcement
// draft; every observable lifecycle transition supplies one.
type MessagePersistence struct {
	Message   domain.Message
	Audiences []domain.AudienceRule
	Recipient *domain.PrivateRecipient
	Event     *domain.MessageEvent
}

// MessageChange updates a single message copy and optionally replaces its
// audience. The caller supplies the next aggregate version and corresponding
// event, so the adapter can enforce ordered Outbox persistence.
type MessageChange struct {
	Message          domain.Message
	ReplaceAudiences *([]domain.AudienceRule)
	Event            domain.MessageEvent
}

type MessageListQuery struct {
	OrganizationIDs     []uint
	Kinds               []domain.MessageKind
	Statuses            []domain.MessageStatus
	CategoryID          uint
	Keyword             string
	PublishAtOnOrBefore *time.Time
	ExpiresAtOnOrBefore *time.Time
	Offset, Limit       int
}

type InboxStore interface {
	ListInbox(context.Context, InboxQuery) ([]InboxMessage, int64, error)
	FindInboxMessage(context.Context, InboxIdentity) (InboxMessage, bool, error)
	MarkInboxRead(context.Context, InboxIdentity, time.Time) (bool, error)
	DeletePrivateInbox(context.Context, InboxIdentity, time.Time) (bool, error)
	CountUnreadInbox(context.Context, InboxQuery) (UnreadInboxCount, error)
}

// NotificationStore exposes only private recipient IDs needed to build an
// in-process notification projection. Dynamic audiences are resolved through
// OrganizationAudienceReader at projection time.
type NotificationStore interface {
	ListPrivateNotificationUserIDs(context.Context, uint) ([]uint, error)
}

// InboxQuery receives the user's current organization membership and role
// facts from Messaging's Organization and Authorization readers. The
// repository evaluates stored audience rules without importing either module.
// Membership JoinedAt is required to initialize first-visible announcements.
type InboxQuery struct {
	UserID        uint
	Memberships   []OrganizationMembership
	RoleIDs       []uint
	Kinds         []domain.MessageKind
	CategoryID    uint
	Read          *bool
	Keyword       string
	Now           time.Time
	Offset, Limit int
}

type InboxIdentity struct {
	UserID      uint
	MessageID   uint
	Memberships []OrganizationMembership
	RoleIDs     []uint
	Now         time.Time
}

type InboxMessage struct {
	Message        domain.Message
	ReadAt         *time.Time
	PrivateDeleted bool
}

type UnreadInboxCount struct {
	Private, Broadcast, Announcement, Total int64
}

type OutboxStore interface {
	ClaimOutbox(context.Context, OutboxClaim) ([]domain.MessageOutbox, error)
	RenewOutboxLease(context.Context, OutboxLease) (bool, error)
	MarkOutboxPublished(context.Context, OutboxLease) (bool, error)
	RecordOutboxFailure(context.Context, OutboxFailure) (bool, error)
	ReplayOutbox(context.Context, uint) (bool, error)
	ListOutboxes(context.Context, OutboxListQuery) ([]domain.MessageOutbox, int64, error)
}

type OutboxClaim struct {
	WorkerID string
	Now      time.Time
	Lease    time.Duration
	Limit    int
}

type OutboxLease struct {
	ID       uint
	WorkerID string
	Now      time.Time
	Lease    time.Duration
}

type OutboxFailure struct {
	OutboxLease
	FailureCode string
	RetryAt     *time.Time
	Dead        bool
}

type OutboxListQuery struct {
	Statuses        []domain.OutboxStatus
	OrganizationIDs []uint
	Offset, Limit   int
}

type ConsumerStore interface {
	ClaimEventConsumption(context.Context, EventConsumptionClaim) (EventConsumption, bool, error)
	RenewEventConsumptionLease(context.Context, EventConsumptionLease) (bool, error)
	ResetIncompleteAudienceSnapshot(context.Context, EventConsumptionLease) (bool, error)
	PersistAudienceDeliveryBatch(context.Context, AudienceDeliveryBatch) (bool, error)
	DiscardAudienceDeliverySnapshot(context.Context, EventConsumptionLease) (bool, error)
	MarkAudienceSnapshotComplete(context.Context, EventConsumptionLease) (bool, error)
	FailEventConsumption(context.Context, EventConsumptionFailure) (bool, error)
	FinalizeEventConsumption(context.Context, EventConsumptionCompletion) (EventConsumptionFinalization, error)
	ListAudienceDeliveryUserIDs(context.Context, string, string) ([]uint, error)
	CleanupAudienceDeliveries(context.Context, time.Time, int) (int64, error)
}

type EventConsumptionClaim struct {
	ConsumerName string
	Event        domain.MessageEvent
	WorkerID     string
	Now          time.Time
	Lease        time.Duration
}

type EventConsumption struct {
	ConsumerName          string
	EventID               string
	MessageCopyID         uint
	AggregateVersion      uint64
	Status                domain.EventConsumptionStatus
	WorkerID              string
	SnapshotFence         uint64
	SnapshotComplete      bool
	LeaseExpiresAt        *time.Time
	FailureCode           string
	AudienceObservedCount int
	CompletedAt           *time.Time
}

type EventConsumptionLease struct {
	ConsumerName  string
	EventID       string
	WorkerID      string
	SnapshotFence uint64
	Now           time.Time
	Lease         time.Duration
}

// AudienceDeliveryBatch writes a complete snapshot in batches of at most 500
// unique users. The adapter verifies the claim's current fence and lease
// without holding that consumption row locked through the batch transaction.
type AudienceDeliveryBatch struct {
	EventConsumptionLease
	MessageCopyID uint
	UserIDs       []uint
	ExpiresAt     time.Time
}

type EventConsumptionFailure struct {
	EventConsumptionLease
	FailureCode           string
	AudienceObservedCount int
}

type EventConsumptionCompletion struct {
	EventConsumptionLease
	MessageCopyID    uint
	AggregateVersion uint64
}

type EventConsumptionFinalization struct {
	Status    domain.EventConsumptionStatus
	Completed bool
}

type ConsumerDeadLetterStore interface {
	RecordConsumerDeadLetter(context.Context, ConsumerDeadLetterInput) (ConsumerDeadLetter, error)
	ListConsumerDeadLetters(context.Context, ConsumerDeadLetterListQuery) ([]ConsumerDeadLetter, int64, error)
	ClaimConsumerDeadLetterReplay(context.Context, ConsumerDeadLetterReplayClaim) (ConsumerDeadLetter, bool, error)
	MarkConsumerDeadLetterReplayed(context.Context, ConsumerDeadLetterReplayResult) (bool, error)
	ReturnConsumerDeadLetterPending(context.Context, ConsumerDeadLetterReplayResult) (bool, error)
	DiscardConsumerDeadLetter(context.Context, uint, time.Time) (bool, error)
	CleanupFinalConsumerDeadLetters(context.Context, time.Time, int) (int64, error)
}

type ConsumerDeadLetterInput struct {
	ConsumerName          string
	Event                 domain.MessageEvent
	OriginalQueue         string
	RetryAttempt          int
	FailureCode           string
	AudienceObservedCount int
	Invalid               bool
	Fingerprint           string
	Now                   time.Time
}

type ConsumerDeadLetter struct {
	ID                    uint
	ConsumerName          string
	Event                 domain.MessageEvent
	OriginalQueue         string
	RetryAttempt          int
	Status                domain.ConsumerDLQStatus
	ReplayCycle           uint
	ReplayLeaseFence      uint64
	ReplayLeaseOwner      string
	ReplayLeaseExpiresAt  *time.Time
	LastFailureCode       string
	AudienceObservedCount int
	Invalid               bool
	Fingerprint           string
	Replayable            bool
	FinalizedAt           *time.Time
}

type ConsumerDeadLetterListQuery struct {
	Statuses        []domain.ConsumerDLQStatus
	ConsumerName    string
	OrganizationIDs []uint
	Offset, Limit   int
}

type ConsumerDeadLetterReplayClaim struct {
	ID       uint
	WorkerID string
	Now      time.Time
	Lease    time.Duration
}

type ConsumerDeadLetterReplayResult struct {
	ID       uint
	WorkerID string
	Fence    uint64
	Now      time.Time
}
