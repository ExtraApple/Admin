package application_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type notificationMessageStoreFake struct {
	message domain.Message
	rules   []domain.AudienceRule
}

func (store notificationMessageStoreFake) PersistMessage(context.Context, application.MessagePersistence) (domain.Message, error) {
	panic("unused")
}
func (store notificationMessageStoreFake) FindMessage(context.Context, uint) (domain.Message, error) {
	return store.message, nil
}
func (store notificationMessageStoreFake) ListMessages(context.Context, application.MessageListQuery) ([]domain.Message, int64, error) {
	panic("unused")
}
func (store notificationMessageStoreFake) ChangeMessage(context.Context, application.MessageChange) (domain.Message, error) {
	panic("unused")
}
func (store notificationMessageStoreFake) ListAudienceRules(context.Context, uint) ([]domain.AudienceRule, error) {
	return append([]domain.AudienceRule(nil), store.rules...), nil
}

type notificationInboxStoreFake struct {
	items map[uint]application.InboxMessage
}

func (store notificationInboxStoreFake) ListInbox(context.Context, application.InboxQuery) ([]application.InboxMessage, int64, error) {
	panic("unused")
}
func (store notificationInboxStoreFake) FindInboxMessage(_ context.Context, identity application.InboxIdentity) (application.InboxMessage, bool, error) {
	item, ok := store.items[identity.UserID]
	return item, ok, nil
}
func (notificationInboxStoreFake) MarkInboxRead(context.Context, application.InboxIdentity, time.Time) (bool, error) {
	panic("unused")
}
func (notificationInboxStoreFake) DeletePrivateInbox(context.Context, application.InboxIdentity, time.Time) (bool, error) {
	panic("unused")
}
func (notificationInboxStoreFake) CountUnreadInbox(context.Context, application.InboxQuery) (application.UnreadInboxCount, error) {
	panic("unused")
}

type notificationOrganizationFake struct {
	memberships map[uint][]application.OrganizationMembership
	members     map[uint][]uint
	roleMembers map[[2]uint][]uint
}

func (organization notificationOrganizationFake) Memberships(_ context.Context, userID uint) ([]application.OrganizationMembership, error) {
	return append([]application.OrganizationMembership(nil), organization.memberships[userID]...), nil
}
func (notificationOrganizationFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	panic("unused")
}
func (organization notificationOrganizationFake) MemberUserIDs(_ context.Context, organizationID uint) ([]uint, error) {
	return append([]uint(nil), organization.members[organizationID]...), nil
}
func (organization notificationOrganizationFake) RoleMemberUserIDs(_ context.Context, organizationID, roleID uint) ([]uint, error) {
	return append([]uint(nil), organization.roleMembers[[2]uint{organizationID, roleID}]...), nil
}

type notificationIdentityFake struct {
	users map[uint]application.IdentityUser
}

func (identity notificationIdentityFake) LookupUser(_ context.Context, userID uint) (application.IdentityUser, error) {
	return identity.users[userID], nil
}
func (notificationIdentityFake) LookupVerifiedEmail(context.Context, uint) (application.VerifiedEmail, bool, error) {
	panic("unused")
}

type notificationStoreFake struct {
	privateUserIDs []uint
}

func (store notificationStoreFake) ListPrivateNotificationUserIDs(context.Context, uint) ([]uint, error) {
	return append([]uint(nil), store.privateUserIDs...), nil
}

func TestNotificationProjectionReturnsOnlyCurrentUnreadDynamicAudience(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	message := domain.Message{ID: 41, LogicalID: "logical-41", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindBroadcast, Status: domain.MessageStatusPublished, Title: "Release", BodyHTML: "<p>Hello <strong>world</strong></p>", PublishAt: projectionTimePtr(now.Add(-time.Hour)), AggregateVersion: 2}
	service := application.NewService(application.Dependencies{
		Messages: notificationMessageStoreFake{message: message, rules: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}},
		Inbox: notificationInboxStoreFake{items: map[uint]application.InboxMessage{
			7: {Message: message},
			8: {Message: message, ReadAt: projectionTimePtr(now.Add(-time.Minute))},
		}},
		Organizations: notificationOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}, 8: {{OrganizationID: 10}}}, members: map[uint][]uint{10: {7, 8}}},
		Identity:      notificationIdentityFake{users: map[uint]application.IdentityUser{7: {ID: 7, DisplayName: "Alice", Enabled: true}, 8: {ID: 8, DisplayName: "Bob", Enabled: true}}},
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})

	projections, err := service.ProjectNotificationRecipients(context.Background(), application.NotificationProjectionRequest{Event: domain.MessageEvent{EventID: "event-41", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 2}})
	if err != nil || len(projections) != 1 {
		t.Fatalf("ProjectNotificationRecipients() = %#v, %v", projections, err)
	}
	projection := projections[0]
	if projection.UserID != 7 || projection.DisplayName != "Alice" || projection.Title != "Release" || projection.BodyHTML != message.BodyHTML || projection.BodyText != "Hello world" || projection.MessageCopyID != 41 || projection.EventVersion != 1 {
		t.Fatalf("projection = %#v", projection)
	}
}

func TestNotificationProjectionDeduplicatesOverlappingAudienceRulesPerCopy(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	message := domain.Message{ID: 42, LogicalID: "logical-overlap", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindBroadcast, Status: domain.MessageStatusPublished, Title: "Overlap", BodyHTML: "<p>Body</p>", PublishAt: projectionTimePtr(now.Add(-time.Hour)), AggregateVersion: 1}
	service := application.NewService(application.Dependencies{
		Messages: notificationMessageStoreFake{message: message, rules: []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}, {OrganizationID: 10, Type: domain.AudienceTypeRole, RoleID: 3}}},
		Inbox:    notificationInboxStoreFake{items: map[uint]application.InboxMessage{7: {Message: message}, 9: {Message: message}}},
		Organizations: notificationOrganizationFake{
			memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}, 9: {{OrganizationID: 10}}},
			members:     map[uint][]uint{10: {7}},
			roleMembers: map[[2]uint][]uint{{10, 3}: {7, 9}},
		},
		Identity: notificationIdentityFake{users: map[uint]application.IdentityUser{7: {ID: 7, DisplayName: "Alice", Enabled: true}, 9: {ID: 9, DisplayName: "Carol", Enabled: true}}},
		Clock:    application.ClockFunc(func() time.Time { return now }),
	})

	projections, err := service.ProjectNotificationRecipients(context.Background(), application.NotificationProjectionRequest{Event: domain.MessageEvent{EventID: "event-overlap", EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: 42, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}})
	if err != nil || len(projections) != 2 || projections[0].UserID != 7 || projections[1].UserID != 9 {
		t.Fatalf("overlapping projections = %#v, %v", projections, err)
	}
}
func TestNotificationProjectionAllowsPrivateCreatedButRejectsOtherLifecycleEvents(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	message := domain.Message{ID: 41, LogicalID: "logical-private", OrganizationID: 10, SenderID: 7, Kind: domain.MessageKindPrivate, Status: domain.MessageStatusPublished, Title: "Private", BodyHTML: "<p>Body</p>", PublishAt: projectionTimePtr(now.Add(-time.Hour)), AggregateVersion: 1}
	service := application.NewService(application.Dependencies{
		Messages:      notificationMessageStoreFake{message: message},
		Inbox:         notificationInboxStoreFake{items: map[uint]application.InboxMessage{8: {Message: message}}},
		Organizations: notificationOrganizationFake{},
		Identity:      notificationIdentityFake{users: map[uint]application.IdentityUser{8: {ID: 8, DisplayName: "Bob", Enabled: true}}},
		Notifications: notificationStoreFake{privateUserIDs: []uint{8}},
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	created, err := service.ProjectNotificationRecipients(context.Background(), application.NotificationProjectionRequest{Event: domain.MessageEvent{EventID: "event-private", EventName: domain.EventNameMessageCreated, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 1}})
	if err != nil || len(created) != 1 || created[0].UserID != 8 {
		t.Fatalf("private created projection = %#v, %v", created, err)
	}
	ignored, err := service.ProjectNotificationRecipients(context.Background(), application.NotificationProjectionRequest{Event: domain.MessageEvent{EventID: "event-private-revoked", EventName: domain.EventNameMessageRevoked, EventVersion: 1, MessageCopyID: 41, OrganizationID: 10, OccurredAt: now, AggregateVersion: 2}})
	if err != nil || len(ignored) != 0 {
		t.Fatalf("private revoked projection = %#v, %v", ignored, err)
	}
}

func projectionTimePtr(value time.Time) *time.Time { return &value }
