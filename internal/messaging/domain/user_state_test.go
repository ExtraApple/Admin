package domain_test

import (
	"testing"
	"time"

	"admin/internal/messaging/domain"
)

func TestMessageUserStateUnreadAndReadSemantics(t *testing.T) {
	state := domain.MessageUserState{MessageID: 1, UserID: 2}
	if !state.Unread() {
		t.Fatal("message user state without ReadAt must be unread")
	}
	readAt := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	state.ReadAt = &readAt
	if state.Unread() {
		t.Fatal("message user state with ReadAt must be read")
	}
	if err := domain.ValidateMessageUserState(state); err != nil {
		t.Fatalf("ValidateMessageUserState() error = %v", err)
	}
}
func TestAnnouncementBeforeAudienceEntryIsInitiallyRead(t *testing.T) {
	publishedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	enteredBeforePublish := publishedAt.Add(-time.Hour)
	enteredAfterPublish := publishedAt.Add(time.Hour)
	if domain.ShouldInitializeMessageUnread(domain.MessageKindAnnouncement, publishedAt, enteredAfterPublish) {
		t.Fatal("announcement published before audience entry must be initially read")
	}
	if !domain.ShouldInitializeMessageUnread(domain.MessageKindAnnouncement, publishedAt, enteredBeforePublish) {
		t.Fatal("announcement published after audience entry must be initially unread")
	}
	if !domain.ShouldInitializeMessageUnread(domain.MessageKindBroadcast, publishedAt, enteredAfterPublish) {
		t.Fatal("broadcast audience entry must be initially unread")
	}
}

func TestPrivateRecipientDeletionIsAUserScopedTombstone(t *testing.T) {
	recipient := domain.PrivateRecipient{MessageID: 1, SenderID: 2, RecipientID: 3}
	deletedAt := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	if err := domain.MarkPrivateRecipientDeleted(&recipient, deletedAt); err != nil {
		t.Fatalf("MarkPrivateRecipientDeleted() error = %v", err)
	}
	if recipient.DeletedAt == nil || !recipient.DeletedAt.Equal(deletedAt) {
		t.Fatalf("private recipient tombstone = %v, want %v", recipient.DeletedAt, deletedAt)
	}
}

func TestMessageUserStateRejectsPrivateMessages(t *testing.T) {
	state := domain.MessageUserState{MessageID: 1, UserID: 2, Kind: domain.MessageKindPrivate}
	if err := domain.ValidateMessageUserState(state); err != domain.ErrMessageUserStateInvalid {
		t.Fatalf("ValidateMessageUserState() error = %v, want ErrMessageUserStateInvalid", err)
	}
}
