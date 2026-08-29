package domain

import "errors"

type MessageKind string

const (
	MessageKindPrivate      MessageKind = "private"
	MessageKindBroadcast    MessageKind = "broadcast"
	MessageKindAnnouncement MessageKind = "announcement"
)

type MessageStatus string

const (
	MessageStatusDraft     MessageStatus = "draft"
	MessageStatusScheduled MessageStatus = "scheduled"
	MessageStatusPublished MessageStatus = "published"
	MessageStatusExpired   MessageStatus = "expired"
	MessageStatusRevoked   MessageStatus = "revoked"
)

var (
	ErrInvalidMessageKind       = errors.New("invalid message kind")
	ErrInvalidMessageStatus     = errors.New("invalid message status")
	ErrInvalidMessageTransition = errors.New("invalid message status transition")
)

func InitialMessageStatus(kind MessageKind) (MessageStatus, error) {
	switch kind {
	case MessageKindPrivate, MessageKindBroadcast:
		return MessageStatusPublished, nil
	case MessageKindAnnouncement:
		return MessageStatusDraft, nil
	default:
		return "", ErrInvalidMessageKind
	}
}

func CanTransitionMessage(kind MessageKind, from, to MessageStatus) bool {
	if !validMessageKind(kind) || !validMessageStatus(from) || !validMessageStatus(to) || from == to {
		return false
	}
	switch kind {
	case MessageKindPrivate, MessageKindBroadcast:
		return from == MessageStatusPublished && to == MessageStatusRevoked
	case MessageKindAnnouncement:
		switch from {
		case MessageStatusDraft:
			return to == MessageStatusScheduled || to == MessageStatusPublished
		case MessageStatusScheduled:
			return to == MessageStatusPublished
		case MessageStatusPublished:
			return to == MessageStatusExpired || to == MessageStatusRevoked
		default:
			return false
		}
	default:
		return false
	}
}

func TransitionMessage(kind MessageKind, from, to MessageStatus) (MessageStatus, error) {
	if !validMessageKind(kind) {
		return from, ErrInvalidMessageKind
	}
	if !validMessageStatus(from) || !validMessageStatus(to) {
		return from, ErrInvalidMessageStatus
	}
	if !CanTransitionMessage(kind, from, to) {
		return from, ErrInvalidMessageTransition
	}
	return to, nil
}

func validMessageKind(kind MessageKind) bool {
	switch kind {
	case MessageKindPrivate, MessageKindBroadcast, MessageKindAnnouncement:
		return true
	default:
		return false
	}
}

func validMessageStatus(status MessageStatus) bool {
	switch status {
	case MessageStatusDraft, MessageStatusScheduled, MessageStatusPublished, MessageStatusExpired, MessageStatusRevoked:
		return true
	default:
		return false
	}
}
