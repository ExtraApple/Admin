package application_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type inboxStoreFake struct {
	query      application.InboxQuery
	identity   application.InboxIdentity
	countQuery application.InboxQuery
	detail     application.InboxMessage
	visible    bool
}

func (store *inboxStoreFake) ListInbox(_ context.Context, query application.InboxQuery) ([]application.InboxMessage, int64, error) {
	store.query = query
	return []application.InboxMessage{{Message: domain.Message{ID: 7, Kind: domain.MessageKindPrivate}, ReadAt: nil}}, 1, nil
}
func (store *inboxStoreFake) FindInboxMessage(_ context.Context, identity application.InboxIdentity) (application.InboxMessage, bool, error) {
	store.identity = identity
	return store.detail, store.visible, nil
}
func (store *inboxStoreFake) MarkInboxRead(_ context.Context, identity application.InboxIdentity, _ time.Time) (bool, error) {
	store.identity = identity
	return true, nil
}
func (store *inboxStoreFake) DeletePrivateInbox(_ context.Context, identity application.InboxIdentity, _ time.Time) (bool, error) {
	store.identity = identity
	return true, nil
}
func (store *inboxStoreFake) CountUnreadInbox(_ context.Context, query application.InboxQuery) (application.UnreadInboxCount, error) {
	store.countQuery = query
	return application.UnreadInboxCount{Private: 2, Broadcast: 3, Announcement: 4, Total: 9}, nil
}

func TestInboxServicePassesCurrentMembershipsFiltersAndPagination(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &inboxStoreFake{}
	service := application.NewService(application.Dependencies{
		Inbox:         store,
		Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}}},
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	result, err := service.ListInbox(context.Background(), application.InboxQueryRequest{UserID: 7, RoleIDs: []uint{3}, Kinds: []domain.MessageKind{domain.MessageKindAnnouncement}, CategoryID: 9, Read: boolPtr(false), Keyword: "release", Offset: 20, Limit: 10})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("ListInbox() = %#v, %v", result, err)
	}
	if store.query.UserID != 7 || store.query.RoleIDs[0] != 3 || store.query.Memberships[0].OrganizationID != 10 || !store.query.Memberships[0].JoinedAt.Equal(now.Add(-time.Hour)) || store.query.Offset != 20 || store.query.Limit != 10 || store.query.CategoryID != 9 || store.query.Keyword != "release" || store.query.Read == nil || *store.query.Read {
		t.Fatalf("inbox query = %#v", store.query)
	}
}
func TestInboxServiceMarksReadAndReturnsUnreadCounts(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &inboxStoreFake{}
	service := application.NewService(application.Dependencies{Inbox: store, Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}}}, Clock: application.ClockFunc(func() time.Time { return now })})
	if err := service.MarkInboxRead(context.Background(), application.InboxIdentityRequest{UserID: 7, MessageID: 41, RoleIDs: []uint{3}}); err != nil {
		t.Fatalf("MarkInboxRead() = %v", err)
	}
	if store.identity.UserID != 7 || store.identity.MessageID != 41 || len(store.identity.RoleIDs) != 1 || store.identity.Memberships[0].OrganizationID != 10 {
		t.Fatalf("read identity = %#v", store.identity)
	}
	counts, err := service.CountUnreadInbox(context.Background(), application.UnreadInboxRequest{UserID: 7, RoleIDs: []uint{3}})
	if err != nil || counts.Total != 9 || counts.Private != 2 || counts.Broadcast != 3 || counts.Announcement != 4 {
		t.Fatalf("CountUnreadInbox() = %#v, %v", counts, err)
	}
}
func TestInboxServiceDeletesOnlyPrivateInboxRelationship(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &inboxStoreFake{}
	service := application.NewService(application.Dependencies{Inbox: store, Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}}}, Clock: application.ClockFunc(func() time.Time { return now })})
	if err := service.DeletePrivateInbox(context.Background(), application.InboxIdentityRequest{UserID: 7, MessageID: 41, RoleIDs: []uint{3}}); err != nil {
		t.Fatalf("DeletePrivateInbox() = %v", err)
	}

	if store.identity.UserID != 7 || store.identity.MessageID != 41 || store.identity.Memberships[0].OrganizationID != 10 {
		t.Fatalf("delete identity = %#v", store.identity)
	}
}
func TestInboxServiceReturnsOnlyCurrentlyVisibleMessageDetails(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &inboxStoreFake{detail: application.InboxMessage{Message: domain.Message{ID: 41, Kind: domain.MessageKindPrivate, Title: "hello"}}, visible: true}
	service := application.NewService(application.Dependencies{
		Inbox:         store,
		Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}}},
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	item, err := service.GetInboxMessage(context.Background(), application.InboxIdentityRequest{UserID: 7, MessageID: 41, RoleIDs: []uint{3}})
	if err != nil || item.Message.ID != 41 || item.Message.Title != "hello" || store.identity.UserID != 7 || store.identity.MessageID != 41 {
		t.Fatalf("GetInboxMessage() = %#v, %v identity=%#v", item, err, store.identity)
	}
}

type roleAwareOrganizationFake struct {
	privateOrganizationFake
	roleIDs []uint
}

func (organization roleAwareOrganizationFake) RoleIDs(context.Context, uint) ([]uint, error) {
	return append([]uint(nil), organization.roleIDs...), nil
}

func TestInboxServiceUsesCurrentRoleIDsInsteadOfCallerFilter(t *testing.T) {
	store := &inboxStoreFake{}
	service := application.NewService(application.Dependencies{Inbox: store, Organizations: roleAwareOrganizationFake{privateOrganizationFake: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}}}, roleIDs: []uint{3}}})
	if _, err := service.ListInbox(context.Background(), application.InboxQueryRequest{UserID: 7, RoleIDs: []uint{999}}); err != nil {
		t.Fatalf("ListInbox() = %v", err)
	}
	if len(store.query.RoleIDs) != 1 || store.query.RoleIDs[0] != 3 {
		t.Fatalf("role filter = %#v, want current role IDs", store.query.RoleIDs)
	}
}

func boolPtr(value bool) *bool { return &value }
