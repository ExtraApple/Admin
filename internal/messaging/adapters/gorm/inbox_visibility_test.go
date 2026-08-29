package gormadapter_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

func TestRepositoryConcealsRevokedBodyAndHidesDeletedPrivateRelation(t *testing.T) {
	repository, _ := openRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	message, err := repository.PersistMessage(ctx, application.MessagePersistence{
		Message:   newMessage("logical-revoked-private", 10, 7, 0, domain.MessageKindPrivate, now),
		Recipient: &domain.PrivateRecipient{SenderID: 7, RecipientID: 8},
		Event:     event("event-private-created", 0, 10, 1, now),
	})
	if err != nil {
		t.Fatalf("PersistMessage() = %v", err)
	}
	revokedAt := now.Add(time.Minute)
	message.Status = domain.MessageStatusRevoked
	message.RevokedAt = &revokedAt
	message.AggregateVersion = 2
	revokeEvent := *event("event-private-revoked", message.ID, 10, 2, revokedAt)
	revokeEvent.EventName = domain.EventNameMessageRevoked
	if _, err := repository.ChangeMessage(ctx, application.MessageChange{Message: message, Event: revokeEvent}); err != nil {
		t.Fatalf("ChangeMessage() = %v", err)
	}

	identity := application.InboxIdentity{UserID: 8, MessageID: message.ID, Memberships: []application.OrganizationMembership{{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}, Now: revokedAt}
	inbox, total, err := repository.ListInbox(ctx, application.InboxQuery{UserID: identity.UserID, Memberships: identity.Memberships, Now: identity.Now})
	if err != nil || total != 1 || len(inbox) != 1 || inbox[0].Message.Status != domain.MessageStatusRevoked || inbox[0].Message.BodyHTML != "" {
		t.Fatalf("revoked inbox = %#v total=%d err=%v", inbox, total, err)
	}
	deleted, err := repository.DeletePrivateInbox(ctx, identity, revokedAt)
	if err != nil || !deleted {
		t.Fatalf("DeletePrivateInbox() = %t, %v", deleted, err)
	}
	inbox, total, err = repository.ListInbox(ctx, application.InboxQuery{UserID: identity.UserID, Memberships: identity.Memberships, Now: identity.Now})
	if err != nil || total != 0 || len(inbox) != 0 {
		t.Fatalf("deleted private inbox = %#v total=%d err=%v", inbox, total, err)
	}
}
