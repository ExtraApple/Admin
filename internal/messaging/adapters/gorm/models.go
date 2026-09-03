package gormadapter

import (
	"time"

	messagingdomain "admin/internal/messaging/domain"
)

type MessageCategory struct {
	ID             uint `gorm:"primaryKey"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	OrganizationID uint   `gorm:"not null;uniqueIndex:ux_message_category_org_code"`
	Code           string `gorm:"type:varchar(64);not null;uniqueIndex:ux_message_category_org_code"`
	Name           string `gorm:"type:varchar(100);not null"`
	Sort           int    `gorm:"not null;default:0"`
	Enabled        bool   `gorm:"not null;default:true;index"`
}

func (MessageCategory) TableName() string { return "message_categories" }

type Message struct {
	ID               uint `gorm:"primaryKey"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LogicalID        string                        `gorm:"type:char(36);not null;index"`
	OrganizationID   uint                          `gorm:"not null;index"`
	SenderID         uint                          `gorm:"not null;index"`
	CategoryID       uint                          `gorm:"index"`
	Kind             messagingdomain.MessageKind   `gorm:"type:varchar(24);not null;index"`
	Status           messagingdomain.MessageStatus `gorm:"type:varchar(24);not null;index"`
	Title            string                        `gorm:"type:varchar(400);not null"`
	BodyHTML         string                        `gorm:"type:longtext;not null"`
	PublishAt        *time.Time                    `gorm:"index"`
	ExpiresAt        *time.Time                    `gorm:"index"`
	RevokedAt        *time.Time
	AggregateVersion uint64 `gorm:"not null;default:1"`
}

func (Message) TableName() string { return "messages" }

type MessageAudience struct {
	ID             uint                         `gorm:"primaryKey"`
	MessageID      uint                         `gorm:"not null;uniqueIndex:ux_message_audience_rule"`
	OrganizationID uint                         `gorm:"not null;uniqueIndex:ux_message_audience_rule"`
	Type           messagingdomain.AudienceType `gorm:"type:varchar(24);not null;uniqueIndex:ux_message_audience_rule"`
	RoleID         uint                         `gorm:"not null;default:0;uniqueIndex:ux_message_audience_rule"`
}

func (MessageAudience) TableName() string { return "message_audiences" }

type MessageRecipient struct {
	ID          uint `gorm:"primaryKey"`
	MessageID   uint `gorm:"not null;uniqueIndex:ux_message_recipient"`
	SenderID    uint `gorm:"not null"`
	RecipientID uint `gorm:"not null;uniqueIndex:ux_message_recipient"`
	ReadAt      *time.Time
	DeletedAt   *time.Time `gorm:"index"`
}

func (MessageRecipient) TableName() string { return "message_recipients" }

type MessageUserState struct {
	ID        uint       `gorm:"primaryKey"`
	MessageID uint       `gorm:"not null;uniqueIndex:ux_message_user_state"`
	UserID    uint       `gorm:"not null;uniqueIndex:ux_message_user_state"`
	ReadAt    *time.Time `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (MessageUserState) TableName() string { return "message_user_states" }

type MessageOutbox struct {
	ID               uint `gorm:"primaryKey"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	EventID          string                       `gorm:"type:char(36);not null;uniqueIndex:ux_message_outbox_event"`
	EventName        messagingdomain.EventName    `gorm:"type:varchar(64);not null"`
	EventVersion     uint                         `gorm:"not null"`
	MessageCopyID    uint                         `gorm:"not null;uniqueIndex:ux_message_outbox_copy_version"`
	OrganizationID   uint                         `gorm:"not null;index"`
	AggregateVersion uint64                       `gorm:"not null;uniqueIndex:ux_message_outbox_copy_version"`
	OccurredAt       time.Time                    `gorm:"not null"`
	Status           messagingdomain.OutboxStatus `gorm:"type:varchar(24);not null;index"`
	RetryAttempt     int                          `gorm:"not null;default:0"`
	NextRetryAt      *time.Time                   `gorm:"index"`
	WorkerID         string                       `gorm:"type:varchar(64);index"`
	LeaseExpiresAt   *time.Time                   `gorm:"index"`
	PublishedAt      *time.Time
	LastFailureCode  string `gorm:"type:varchar(64);index"`
}

func (MessageOutbox) TableName() string { return "message_outboxes" }

type MessageEventConsumption struct {
	ID                    uint `gorm:"primaryKey"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ConsumerName          string                                 `gorm:"type:varchar(64);not null;uniqueIndex:ux_message_consumption_consumer_event"`
	EventID               string                                 `gorm:"type:char(36);not null;uniqueIndex:ux_message_consumption_consumer_event"`
	MessageCopyID         uint                                   `gorm:"not null;index"`
	AggregateVersion      uint64                                 `gorm:"not null"`
	Status                messagingdomain.EventConsumptionStatus `gorm:"type:varchar(24);not null;index"`
	WorkerID              string                                 `gorm:"type:varchar(64);index"`
	SnapshotFence         uint64                                 `gorm:"not null;default:0"`
	SnapshotComplete      bool                                   `gorm:"not null;default:false"`
	LeaseExpiresAt        *time.Time                             `gorm:"index"`
	FailureCode           string                                 `gorm:"type:varchar(64);index"`
	AudienceObservedCount int                                    `gorm:"not null;default:0"`
	CompletedAt           *time.Time
}

func (MessageEventConsumption) TableName() string { return "message_event_consumptions" }

type MessageEventConsumerCursor struct {
	ID               uint `gorm:"primaryKey"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ConsumerName     string `gorm:"type:varchar(64);not null;uniqueIndex:ux_message_cursor_consumer_copy"`
	MessageCopyID    uint   `gorm:"not null;uniqueIndex:ux_message_cursor_consumer_copy"`
	AggregateVersion uint64 `gorm:"not null"`
}

func (MessageEventConsumerCursor) TableName() string { return "message_event_consumer_cursors" }

type MessageEventDelivery struct {
	ID                   uint `gorm:"primaryKey"`
	CreatedAt            time.Time
	ConsumerName         string    `gorm:"type:varchar(64);not null;uniqueIndex:ux_message_delivery_consumer_event_user"`
	EventID              string    `gorm:"type:char(36);not null;uniqueIndex:ux_message_delivery_consumer_event_user"`
	MessageCopyID        uint      `gorm:"not null;index"`
	UserID               uint      `gorm:"not null;uniqueIndex:ux_message_delivery_consumer_event_user"`
	ConsumerDeadLetterID *uint     `gorm:"index"`
	ExpiresAt            time.Time `gorm:"not null;index"`
}

func (MessageEventDelivery) TableName() string { return "message_event_deliveries" }

type MessageConsumerDeadLetter struct {
	ID                    uint `gorm:"primaryKey"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ConsumerName          string                            `gorm:"type:varchar(64);not null;uniqueIndex:ux_message_dead_letter_consumer_event"`
	EventID               string                            `gorm:"type:varchar(128);not null;uniqueIndex:ux_message_dead_letter_consumer_event"`
	OriginalQueue         string                            `gorm:"type:varchar(128);not null"`
	EventName             messagingdomain.EventName         `gorm:"type:varchar(64);not null"`
	EventVersion          uint                              `gorm:"not null"`
	OccurredAt            time.Time                         `gorm:"index"`
	MessageCopyID         uint                              `gorm:"not null;index"`
	OrganizationID        uint                              `gorm:"not null;index"`
	AggregateVersion      uint64                            `gorm:"not null"`
	RetryAttempt          int                               `gorm:"not null"`
	Status                messagingdomain.ConsumerDLQStatus `gorm:"type:varchar(24);not null;index"`
	ReplayCycle           uint                              `gorm:"not null;default:0"`
	ReplayLeaseFence      uint64                            `gorm:"not null;default:0"`
	ReplayLeaseOwner      string                            `gorm:"type:varchar(64);index"`
	ReplayLeaseExpiresAt  *time.Time                        `gorm:"index"`
	LastFailureCode       string                            `gorm:"type:varchar(64);not null;index"`
	AudienceObservedCount int                               `gorm:"not null;default:0"`
	Invalid               bool                              `gorm:"not null;default:false;index"`
	Fingerprint           string                            `gorm:"type:char(64);not null;index"`
	Replayable            bool                              `gorm:"not null;index"`
	FinalizedAt           *time.Time                        `gorm:"index"`
}

func (MessageConsumerDeadLetter) TableName() string { return "message_consumer_dead_letters" }

func Models() []any {
	return []any{
		MessageCategory{},
		Message{},
		MessageAudience{},
		MessageRecipient{},
		MessageUserState{},
		MessageOutbox{},
		MessageEventConsumption{},
		MessageEventConsumerCursor{},
		MessageEventDelivery{},
		MessageConsumerDeadLetter{},
	}
}
