package application_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type revokeMessageStoreFake struct {
	message domain.Message
	change  application.MessageChange
}

func (store *revokeMessageStoreFake) PersistMessage(context.Context, application.MessagePersistence) (domain.Message, error) {
	panic("unused")
}
func (store *revokeMessageStoreFake) FindMessage(context.Context, uint) (domain.Message, error) {
	return store.message, nil
}
func (store *revokeMessageStoreFake) ListMessages(context.Context, application.MessageListQuery) ([]domain.Message, int64, error) {
	panic("unused")
}
func (store *revokeMessageStoreFake) ChangeMessage(_ context.Context, change application.MessageChange) (domain.Message, error) {
	store.change = change
	store.message = change.Message
	return change.Message, nil
}
func (store *revokeMessageStoreFake) ListAudienceRules(context.Context, uint) ([]domain.AudienceRule, error) {
	panic("unused")
}

func TestPrivateMessageServiceRevokesOnlyOwnPrivateMessage(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &revokeMessageStoreFake{message: domain.Message{ID: 41, LogicalID: "logical-1", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindPrivate, Status: domain.MessageStatusPublished, Title: "title", BodyHTML: "<p>body</p>", PublishAt: messageTimePtr(now.Add(-time.Hour)), AggregateVersion: 1}}
	service := application.NewService(application.Dependencies{Messages: store, Clock: application.ClockFunc(func() time.Time { return now })})
	message, err := service.RevokeOwnPrivateMessage(context.Background(), application.RevokeMessageRequest{ActorID: 7, MessageID: 41})
	if err != nil {
		t.Fatalf("RevokeOwnPrivateMessage() = %v", err)
	}
	if message.Status != domain.MessageStatusRevoked || message.RevokedAt == nil || !message.RevokedAt.Equal(now) || message.AggregateVersion != 2 {
		t.Fatalf("revoked message = %#v", message)
	}
	if store.change.Event.EventName != domain.EventNameMessageRevoked || store.change.Event.AggregateVersion != 2 || store.change.Event.MessageCopyID != 41 {
		t.Fatalf("revoke change = %#v", store.change)
	}
}

func TestPrivateMessageServiceRejectsRevokingAnotherSenderMessage(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &revokeMessageStoreFake{message: domain.Message{ID: 41, LogicalID: "logical-1", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindPrivate, Status: domain.MessageStatusPublished, Title: "title", BodyHTML: "<p>body</p>", PublishAt: messageTimePtr(now.Add(-time.Hour)), AggregateVersion: 1}}
	service := application.NewService(application.Dependencies{Messages: store, Clock: application.ClockFunc(func() time.Time { return now })})
	if _, err := service.RevokeOwnPrivateMessage(context.Background(), application.RevokeMessageRequest{ActorID: 8, MessageID: 41}); err != application.ErrRevokeNotAllowed {
		t.Fatalf("revoke error = %v", err)
	}
	if store.change.Event.EventID != "" {
		t.Fatal("unauthorized revoke was persisted")
	}
}

func messageTimePtr(value time.Time) *time.Time { return &value }
