package application_test

import (
	"context"
	"testing"
	"time"

	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

type revocationAuthorizationFake struct {
	scopes map[uint]application.MessageOrganizationScope
}

func (reader revocationAuthorizationFake) HasPermission(context.Context, uint, string) (bool, error) {
	return true, nil
}

func (reader revocationAuthorizationFake) OrganizationScope(_ context.Context, userID uint) (application.MessageOrganizationScope, error) {
	return reader.scopes[userID], nil
}

func TestManagedRevocationAllowsOrganizationAdminsOnlyForOrdinarySendersAndSuperAdminsGlobally(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := application.NewService(application.Dependencies{
		Messages:      repository,
		Authorization: revocationAuthorizationFake{scopes: map[uint]application.MessageOrganizationScope{1: {All: true}, 7: {OrganizationIDs: []uint{10}}, 8: {}, 9: {OrganizationIDs: []uint{10}}, 10: {All: true}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	ordinary := persistPrivateForRevocation(t, repository, "ordinary-private", 8, 11, now)
	revoked, err := service.RevokeManagedMessage(context.Background(), application.ManagedRevokeRequest{ActorID: 7, MessageID: ordinary.ID})
	if err != nil || revoked.Status != domain.MessageStatusRevoked || revoked.AggregateVersion != 2 || revoked.RevokedAt == nil {
		t.Fatalf("organization-admin revoke = %#v, %v", revoked, err)
	}

	administrator := persistPrivateForRevocation(t, repository, "administrator-private", 9, 11, now)
	if _, err := service.RevokeManagedMessage(context.Background(), application.ManagedRevokeRequest{ActorID: 7, MessageID: administrator.ID}); err != application.ErrRevokeNotAllowed {
		t.Fatalf("organization-admin revoke administrator error = %v, want ErrRevokeNotAllowed", err)
	}
	superAdministrator := persistPrivateForRevocation(t, repository, "super-private", 10, 11, now)
	if revoked, err := service.RevokeManagedMessage(context.Background(), application.ManagedRevokeRequest{ActorID: 1, MessageID: superAdministrator.ID}); err != nil || revoked.Status != domain.MessageStatusRevoked {
		t.Fatalf("super-admin revoke = %#v, %v", revoked, err)
	}
}

func TestManagedRevocationAllowsAnnouncementPublisherButNotOtherAdministrator(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	service := application.NewService(application.Dependencies{
		Messages:      repository,
		Authorization: revocationAuthorizationFake{scopes: map[uint]application.MessageOrganizationScope{1: {All: true}, 7: {OrganizationIDs: []uint{10}}, 9: {OrganizationIDs: []uint{10}}}},
		Organizations: adminOrganizationFake{descendants: []uint{10}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	own := persistAnnouncementForRevocation(t, repository, "own-announcement", 7, now)
	if revoked, err := service.RevokeManagedMessage(context.Background(), application.ManagedRevokeRequest{ActorID: 7, MessageID: own.ID}); err != nil || revoked.Status != domain.MessageStatusRevoked {
		t.Fatalf("announcement publisher revoke = %#v, %v", revoked, err)
	}

	other := persistAnnouncementForRevocation(t, repository, "other-announcement", 9, now)
	if _, err := service.RevokeManagedMessage(context.Background(), application.ManagedRevokeRequest{ActorID: 7, MessageID: other.ID}); err != application.ErrRevokeNotAllowed {
		t.Fatalf("organization-admin revoke other announcement error = %v, want ErrRevokeNotAllowed", err)
	}
}

func persistAnnouncementForRevocation(t *testing.T, repository *messaginggorm.Repository, logicalID string, senderID uint, now time.Time) domain.Message {
	t.Helper()
	message, err := repository.PersistMessage(context.Background(), application.MessagePersistence{
		Message:   domain.Message{LogicalID: logicalID, OrganizationID: 10, SenderID: senderID, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusPublished, Title: "Announcement", BodyHTML: "<p>Body</p>", PublishAt: &now, AggregateVersion: 1},
		Audiences: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}},
		Event:     &domain.MessageEvent{EventID: logicalID + "-event", EventName: domain.EventNameMessagePublished, EventVersion: 1, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1},
	})
	if err != nil {
		t.Fatalf("persist announcement: %v", err)
	}
	return message
}

func persistPrivateForRevocation(t *testing.T, repository *messaginggorm.Repository, logicalID string, senderID, recipientID uint, now time.Time) domain.Message {
	t.Helper()
	message, err := repository.PersistMessage(context.Background(), application.MessagePersistence{Message: domain.Message{LogicalID: logicalID, OrganizationID: 10, SenderID: senderID, Kind: domain.MessageKindPrivate, Status: domain.MessageStatusPublished, Title: "Private", BodyHTML: "<p>Body</p>", PublishAt: &now, AggregateVersion: 1}, Recipient: &domain.PrivateRecipient{SenderID: senderID, RecipientID: recipientID}, Event: &domain.MessageEvent{EventID: logicalID + "-event", EventName: domain.EventNameMessageCreated, EventVersion: 1, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}})
	if err != nil {
		t.Fatalf("persist private message: %v", err)
	}
	return message
}
