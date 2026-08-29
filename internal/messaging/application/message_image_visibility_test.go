package application_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type visibleImageInboxFake struct {
	item     application.InboxMessage
	visible  bool
	identity application.InboxIdentity
}

func (store *visibleImageInboxFake) ListInbox(context.Context, application.InboxQuery) ([]application.InboxMessage, int64, error) {
	panic("unused")
}

func (store *visibleImageInboxFake) FindInboxMessage(_ context.Context, identity application.InboxIdentity) (application.InboxMessage, bool, error) {
	store.identity = identity
	return store.item, store.visible, nil
}

func (store *visibleImageInboxFake) MarkInboxRead(context.Context, application.InboxIdentity, time.Time) (bool, error) {
	panic("unused")
}

func (store *visibleImageInboxFake) DeletePrivateInbox(context.Context, application.InboxIdentity, time.Time) (bool, error) {
	panic("unused")
}

func (store *visibleImageInboxFake) CountUnreadInbox(context.Context, application.InboxQuery) (application.UnreadInboxCount, error) {
	panic("unused")
}

type visibleMessageImageFilesFake struct {
	request application.VisibleMessageImage
	calls   int
}

func (files *visibleMessageImageFilesFake) UploadMessageImage(context.Context, application.MessageImageUpload) (application.TemporaryMessageImage, error) {
	panic("unused")
}

func (files *visibleMessageImageFilesFake) BindMessageImages(context.Context, application.MessageImageBinding) error {
	panic("unused")
}

func (files *visibleMessageImageFilesFake) OpenMessageImage(_ context.Context, request application.VisibleMessageImage) (application.MessageImageContent, error) {
	files.request = request
	files.calls++
	return application.MessageImageContent{Reader: io.NopCloser(strings.NewReader("image")), ContentType: "image/png", Size: 5}, nil
}

func TestMessageImageReadRequiresCurrentPublishedMessageVisibility(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	inbox := &visibleImageInboxFake{item: application.InboxMessage{Message: domain.Message{ID: 41, LogicalID: "message-logical-id", Status: domain.MessageStatusPublished}}, visible: true}
	files := &visibleMessageImageFilesFake{}
	service := application.NewService(application.Dependencies{
		Inbox:         inbox,
		Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}}},
		Files:         files,
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})

	content, err := service.OpenVisibleMessageImage(context.Background(), application.MessageImageReadRequest{UserID: 7, MessageID: 41, ImageID: 9, RoleIDs: []uint{3}})
	if err != nil || content.ContentType != "image/png" || files.calls != 1 {
		t.Fatalf("OpenVisibleMessageImage() = %#v, %v; calls=%d", content, err, files.calls)
	}
	defer content.Reader.Close()
	if files.request != (application.VisibleMessageImage{ID: 9, MessageLogicalID: "message-logical-id"}) {
		t.Fatalf("image request = %#v", files.request)
	}
	if inbox.identity.UserID != 7 || inbox.identity.MessageID != 41 || inbox.identity.Memberships[0].OrganizationID != 10 {
		t.Fatalf("inbox identity = %#v", inbox.identity)
	}
}

func TestMessageImageReadRejectsInvisibleOrRevokedMessage(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	for name, inbox := range map[string]*visibleImageInboxFake{
		"invisible": {visible: false},
		"revoked":   {item: application.InboxMessage{Message: domain.Message{ID: 41, LogicalID: "message-logical-id", Status: domain.MessageStatusRevoked}}, visible: true},
	} {
		t.Run(name, func(t *testing.T) {
			files := &visibleMessageImageFilesFake{}
			service := application.NewService(application.Dependencies{
				Inbox:         inbox,
				Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10, JoinedAt: now.Add(-time.Hour)}}}},
				Files:         files,
				Clock:         application.ClockFunc(func() time.Time { return now }),
			})
			if _, err := service.OpenVisibleMessageImage(context.Background(), application.MessageImageReadRequest{UserID: 7, MessageID: 41, ImageID: 9}); err != application.ErrNotFound {
				t.Fatalf("OpenVisibleMessageImage() error = %v, want ErrNotFound", err)
			}
			if files.calls != 0 {
				t.Fatalf("opened image for %s message", name)
			}
		})
	}
}
