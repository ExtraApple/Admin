package domain

import (
	"errors"
	"strings"
	"time"
)

type Message struct {
	ID               uint
	LogicalID        string
	OrganizationID   uint
	SenderID         uint
	CategoryID       uint
	Kind             MessageKind
	Status           MessageStatus
	Title            string
	BodyHTML         string
	PublishAt        *time.Time
	ExpiresAt        *time.Time
	RevokedAt        *time.Time
	AggregateVersion uint64
}

type MessageCategory struct {
	ID             uint
	OrganizationID uint
	Code           string
	Name           string
	Sort           int
	Enabled        bool
}

type AudienceType string

const (
	AudienceTypeOrganization AudienceType = "organization"
	AudienceTypeRole         AudienceType = "role"
	AudienceTypeAll          AudienceType = "all"
)

type AudienceRule struct {
	Type           AudienceType
	OrganizationID uint
	RoleID         uint
}

type PrivateRecipient struct {
	MessageID   uint
	SenderID    uint
	RecipientID uint
	ReadAt      *time.Time
	DeletedAt   *time.Time
}

var (
	ErrMessageIdentityInvalid  = errors.New("message identity is invalid")
	ErrMessageContentEmpty     = errors.New("message content is empty")
	ErrMessageStatusInvalid    = errors.New("message status is invalid for message kind")
	ErrMessageDatesInvalid     = errors.New("message dates are invalid")
	ErrAudienceRuleInvalid     = errors.New("message audience rule is invalid")
	ErrPrivateRecipientInvalid = errors.New("private recipient is invalid")
)

func ValidateMessage(message Message) error {
	if message.LogicalID == "" || message.OrganizationID == 0 || message.SenderID == 0 {
		return ErrMessageIdentityInvalid
	}
	if strings.TrimSpace(message.Title) == "" || strings.TrimSpace(message.BodyHTML) == "" {
		return ErrMessageContentEmpty
	}
	if _, err := InitialMessageStatus(message.Kind); err != nil {
		return err
	}
	if !validMessageStatus(message.Status) {
		return ErrInvalidMessageStatus
	}
	if (message.Kind == MessageKindPrivate || message.Kind == MessageKindBroadcast) && message.Status != MessageStatusPublished && message.Status != MessageStatusRevoked {
		return ErrMessageStatusInvalid
	}
	if message.ExpiresAt != nil && message.PublishAt != nil && message.ExpiresAt.Before(*message.PublishAt) {
		return ErrMessageDatesInvalid
	}
	if message.Status == MessageStatusRevoked && message.RevokedAt == nil {
		return ErrMessageDatesInvalid
	}
	return nil
}

func ValidateMessageCategory(category MessageCategory) error {
	if category.OrganizationID == 0 || strings.TrimSpace(category.Code) == "" || strings.TrimSpace(category.Name) == "" {
		return ErrMessageIdentityInvalid
	}
	return nil
}

func ValidateAudienceRule(rule AudienceRule) error {
	if rule.OrganizationID == 0 {
		return ErrAudienceRuleInvalid
	}
	switch rule.Type {
	case AudienceTypeOrganization, AudienceTypeAll:
		return nil
	case AudienceTypeRole:
		if rule.RoleID == 0 {
			return ErrAudienceRuleInvalid
		}
		return nil
	default:
		return ErrAudienceRuleInvalid
	}
}

func ValidatePrivateRecipient(recipient PrivateRecipient) error {
	if recipient.MessageID == 0 || recipient.SenderID == 0 || recipient.RecipientID == 0 || recipient.SenderID == recipient.RecipientID {
		return ErrPrivateRecipientInvalid
	}
	return nil
}
