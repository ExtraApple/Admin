package application_test

import (
	"context"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

type privateMessageStoreFake struct {
	persistence application.MessagePersistence
	calls       int
}

func (store *privateMessageStoreFake) PersistMessage(_ context.Context, persistence application.MessagePersistence) (domain.Message, error) {
	store.calls++
	store.persistence = persistence
	persistence.Message.ID = 41
	return persistence.Message, nil
}

func (store *privateMessageStoreFake) FindMessage(context.Context, uint) (domain.Message, error) {
	panic("unused")
}
func (store *privateMessageStoreFake) ListMessages(context.Context, application.MessageListQuery) ([]domain.Message, int64, error) {
	panic("unused")
}
func (store *privateMessageStoreFake) ChangeMessage(context.Context, application.MessageChange) (domain.Message, error) {
	panic("unused")
}
func (store *privateMessageStoreFake) ListAudienceRules(context.Context, uint) ([]domain.AudienceRule, error) {
	panic("unused")
}

func (store *privateMessageStoreFake) _messageStore() application.MessageStore { return store }

type privateMessageFilesFake struct {
	binding application.MessageImageBinding
}

func (*privateMessageFilesFake) UploadMessageImage(context.Context, application.MessageImageUpload) (application.TemporaryMessageImage, error) {
	panic("unused")
}
func (files *privateMessageFilesFake) BindMessageImages(_ context.Context, binding application.MessageImageBinding) error {
	files.binding = binding
	return nil
}
func (*privateMessageFilesFake) OpenMessageImage(context.Context, application.VisibleMessageImage) (application.MessageImageContent, error) {
	panic("unused")
}

func TestPrivateMessageServiceBindsReferencedImagesInMessageOperation(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &privateMessageStoreFake{}
	files := &privateMessageFilesFake{}
	service := application.NewService(application.Dependencies{
		Messages:      store,
		Identity:      privateIdentityFake{users: map[uint]application.IdentityUser{7: {ID: 7, Enabled: true}, 8: {ID: 8, Enabled: true}}},
		Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}, 8: {{OrganizationID: 10}}}},
		Files:         files,
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	message, err := service.SendPrivateMessage(context.Background(), application.SendPrivateMessageRequest{SenderID: 7, RecipientID: 8, Title: "hello", Markdown: "body", ImageIDs: []uint{91, 92}})
	if err != nil {
		t.Fatalf("SendPrivateMessage() = %v", err)
	}
	if files.binding.ActorID != 7 || files.binding.MessageLogicalID != message.LogicalID || len(files.binding.ImageIDs) != 2 || files.binding.ImageIDs[0] != 91 || files.binding.ImageIDs[1] != 92 {
		t.Fatalf("image binding = %#v", files.binding)
	}
}

type privateIdentityFake struct {
	users map[uint]application.IdentityUser
}

func (identity privateIdentityFake) LookupUser(_ context.Context, id uint) (application.IdentityUser, error) {
	user, ok := identity.users[id]
	if !ok {
		return application.IdentityUser{}, application.ErrRecipientUnavailable
	}
	return user, nil
}
func (privateIdentityFake) LookupVerifiedEmail(context.Context, uint) (application.VerifiedEmail, bool, error) {
	return application.VerifiedEmail{}, false, nil
}

type privateOrganizationFake struct {
	memberships map[uint][]application.OrganizationMembership
}

func (organization privateOrganizationFake) Memberships(_ context.Context, userID uint) ([]application.OrganizationMembership, error) {
	return organization.memberships[userID], nil
}
func (privateOrganizationFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	panic("unused")
}
func (privateOrganizationFake) MemberUserIDs(context.Context, uint) ([]uint, error) {
	panic("unused")
}
func (privateOrganizationFake) RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error) {
	panic("unused")
}

func TestPrivateMessageServicePersistsSharedOrganizationAndOutbox(t *testing.T) {
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := &privateMessageStoreFake{}
	service := application.NewService(application.Dependencies{
		Messages:      store,
		Identity:      privateIdentityFake{users: map[uint]application.IdentityUser{7: {ID: 7, Enabled: true}, 8: {ID: 8, Enabled: true}}},
		Organizations: privateOrganizationFake{memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}, {OrganizationID: 20}}, 8: {{OrganizationID: 20}}}},
		Clock:         application.ClockFunc(func() time.Time { return now }),
	})
	message, err := service.SendPrivateMessage(context.Background(), application.SendPrivateMessageRequest{SenderID: 7, RecipientID: 8, Title: "hello", Markdown: "body"})
	if err != nil {
		t.Fatalf("SendPrivateMessage() = %v", err)
	}
	if store.calls != 1 || message.ID != 41 || message.OrganizationID != 20 || message.Kind != domain.MessageKindPrivate || message.Status != domain.MessageStatusPublished || message.BodyHTML == "body" {
		t.Fatalf("message=%#v calls=%d persistence=%#v", message, store.calls, store.persistence)
	}
	if store.persistence.Recipient == nil || store.persistence.Recipient.RecipientID != 8 || store.persistence.Event == nil || store.persistence.Event.EventName != domain.EventNameMessageCreated {
		t.Fatalf("private persistence=%#v", store.persistence)
	}
}

func TestPrivateMessageServiceRejectsUnavailableOrDifferentOrganizationRecipient(t *testing.T) {
	baseIdentity := privateIdentityFake{users: map[uint]application.IdentityUser{7: {ID: 7, Enabled: true}, 8: {ID: 8, Enabled: true}, 9: {ID: 9, Enabled: false}}}
	for _, test := range []struct {
		name        string
		recipientID uint
		memberships map[uint][]application.OrganizationMembership
	}{
		{name: "different organization", recipientID: 8, memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}, 8: {{OrganizationID: 20}}}},
		{name: "disabled recipient", recipientID: 9, memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}, 9: {{OrganizationID: 10}}}},
		{name: "self", recipientID: 7, memberships: map[uint][]application.OrganizationMembership{7: {{OrganizationID: 10}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &privateMessageStoreFake{}
			service := application.NewService(application.Dependencies{Messages: store, Identity: baseIdentity, Organizations: privateOrganizationFake{memberships: test.memberships}, Clock: application.ClockFunc(time.Now)})
			if _, err := service.SendPrivateMessage(context.Background(), application.SendPrivateMessageRequest{SenderID: 7, RecipientID: test.recipientID, Title: "hello", Markdown: "body"}); err != application.ErrRecipientUnavailable {
				t.Fatalf("SendPrivateMessage() error = %v", err)
			}
			if store.calls != 0 {
				t.Fatal("invalid private message reached persistence")
			}
		})
	}
}
