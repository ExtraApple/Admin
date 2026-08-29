package domain

import "time"

type MessageUserState struct {
	MessageID uint
	UserID    uint
	Kind      MessageKind
	ReadAt    *time.Time
}

var ErrMessageUserStateInvalid = errMessageUserStateInvalid()

func errMessageUserStateInvalid() error {
	return &messageUserStateError{}
}

type messageUserStateError struct{}

func (*messageUserStateError) Error() string { return "message user state is invalid" }

func (state MessageUserState) Unread() bool {
	return state.ReadAt == nil
}

func ValidateMessageUserState(state MessageUserState) error {
	if state.MessageID == 0 || state.UserID == 0 {
		return ErrMessageUserStateInvalid
	}
	switch state.Kind {
	case "", MessageKindBroadcast, MessageKindAnnouncement:
		return nil
	default:
		return ErrMessageUserStateInvalid
	}
}

func ShouldInitializeMessageUnread(kind MessageKind, publishedAt, audienceEnteredAt time.Time) bool {
	if kind != MessageKindAnnouncement {
		return true
	}
	return !publishedAt.Before(audienceEnteredAt)
}

func MarkPrivateRecipientDeleted(recipient *PrivateRecipient, deletedAt time.Time) error {
	if recipient == nil || ValidatePrivateRecipient(*recipient) != nil || deletedAt.IsZero() {
		return ErrPrivateRecipientInvalid
	}
	recipient.DeletedAt = &deletedAt
	return nil
}
