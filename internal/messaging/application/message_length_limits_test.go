package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	messaginggorm "admin/internal/messaging/adapters/gorm"
	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	platformdatabase "admin/internal/platform/database"
	"admin/testsupport/testutil"
)

const (
	raisedTitleRunes = 120
	raisedBodyRunes  = 25_000

	lengthLimitOrganizationID = 10
)

func raisedContentLimits() domain.ContentLimits {
	return domain.ContentLimits{MaxTitleRunes: raisedTitleRunes, MaxBodyRunes: raisedBodyRunes}
}

func lengthLimitOrganizationTargets() []application.DynamicAudienceTarget {
	return []application.DynamicAudienceTarget{{OrganizationID: lengthLimitOrganizationID, Type: domain.AudienceTypeOrganization}}
}

// lengthLimitOrganizationFake serves both the private-message membership check
// and the broadcast/announcement audience resolution for one organization.
type lengthLimitOrganizationFake struct{}

func (lengthLimitOrganizationFake) Memberships(context.Context, uint) ([]application.OrganizationMembership, error) {
	return []application.OrganizationMembership{{OrganizationID: lengthLimitOrganizationID, JoinedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}}, nil
}

func (lengthLimitOrganizationFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	return []uint{lengthLimitOrganizationID}, nil
}

func (lengthLimitOrganizationFake) MemberUserIDs(context.Context, uint) ([]uint, error) {
	return nil, nil
}

func (lengthLimitOrganizationFake) RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error) {
	return nil, nil
}

// newLengthLimitService builds one service over an isolated database so every
// content-compiling path is exercised with the same injected limits.
func newLengthLimitService(t *testing.T, limits *domain.ContentLimits) *application.Service {
	t.Helper()
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(messaginggorm.Models()...); err != nil {
		t.Fatalf("migrate Messaging models: %v", err)
	}
	repository := messaginggorm.NewRepository(db)
	if _, err := repository.CreateCategory(context.Background(), domain.MessageCategory{OrganizationID: lengthLimitOrganizationID, Code: "notice", Name: "Notice", Enabled: true}); err != nil {
		t.Fatalf("create category: %v", err)
	}
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	dependencies := application.Dependencies{
		Messages:      repository,
		Categories:    repository,
		Identity:      privateIdentityFake{users: map[uint]application.IdentityUser{7: {ID: 7, Enabled: true}, 8: {ID: 8, Enabled: true}}},
		Organizations: lengthLimitOrganizationFake{},
		Authorization: categoryAuthorizationFake{allowed: true, scope: application.MessageOrganizationScope{OrganizationIDs: []uint{lengthLimitOrganizationID}}},
		Transactions:  platformdatabase.NewTransactionRunner(db),
		Clock:         application.ClockFunc(func() time.Time { return now }),
	}
	if limits != nil {
		dependencies.ContentLimits = *limits
	}
	return application.NewService(dependencies)
}

type messageLengthPath struct {
	name  string
	apply func(t *testing.T, service *application.Service, title, markdown string) error
}

// The four paths that compile message content. Each one must honour the
// injected limits, so a missed call site shows up as a failing subtest.
func messageLengthPaths() []messageLengthPath {
	return []messageLengthPath{
		{name: "CreateAnnouncement", apply: func(_ *testing.T, service *application.Service, title, markdown string) error {
			_, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{
				ActorID: 7, CategoryCode: "notice", Title: title, Markdown: markdown,
				Targets: lengthLimitOrganizationTargets(),
			})
			return err
		}},
		{name: "CreateBroadcast", apply: func(_ *testing.T, service *application.Service, title, markdown string) error {
			_, err := service.CreateBroadcast(context.Background(), application.CreateBroadcastRequest{
				ActorID: 7, CategoryCode: "notice", Title: title, Markdown: markdown,
				Targets: lengthLimitOrganizationTargets(),
			})
			return err
		}},
		{name: "SendPrivateMessage", apply: func(_ *testing.T, service *application.Service, title, markdown string) error {
			_, err := service.SendPrivateMessage(context.Background(), application.SendPrivateMessageRequest{
				SenderID: 7, RecipientID: 8, Title: title, Markdown: markdown,
			})
			return err
		}},
		{name: "EditAnnouncement", apply: func(t *testing.T, service *application.Service, title, markdown string) error {
			t.Helper()
			created, err := service.CreateAnnouncement(context.Background(), application.CreateAnnouncementRequest{
				ActorID: 7, CategoryCode: "notice", Title: "Original", Markdown: "Body",
				Targets: lengthLimitOrganizationTargets(),
			})
			if err != nil || len(created) != 1 {
				t.Fatalf("CreateAnnouncement() = %#v, %v", created, err)
			}
			if _, err := service.PublishAnnouncement(context.Background(), application.PublishAnnouncementRequest{ActorID: 7, MessageID: created[0].ID}); err != nil {
				t.Fatalf("PublishAnnouncement() = %v", err)
			}
			_, err = service.EditAnnouncement(context.Background(), application.EditAnnouncementRequest{
				ActorID: 7, MessageID: created[0].ID, Title: title, Markdown: markdown,
				Targets: lengthLimitOrganizationTargets(),
			})
			return err
		}},
	}
}

func TestServiceUsesDomainDefaultLimitsWhenNoneAreInjected(t *testing.T) {
	service := newLengthLimitService(t, nil)
	ctx := context.Background()
	request := application.SendPrivateMessageRequest{SenderID: 7, RecipientID: 8}

	request.Title = strings.Repeat("标", domain.MaxMessageTitleRunes)
	request.Markdown = strings.Repeat("文", domain.MaxMessageBodyRunes)
	if _, err := service.SendPrivateMessage(ctx, request); err != nil {
		t.Fatalf("message exactly at the default limits was rejected: %v", err)
	}

	request.Title = strings.Repeat("标", domain.MaxMessageTitleRunes+1)
	request.Markdown = "Body"
	if _, err := service.SendPrivateMessage(ctx, request); !errors.Is(err, domain.ErrMessageTitleTooLong) {
		t.Fatalf("title over the default limit error = %v", err)
	}

	request.Title = "标题"
	request.Markdown = strings.Repeat("文", domain.MaxMessageBodyRunes+1)
	if _, err := service.SendPrivateMessage(ctx, request); !errors.Is(err, domain.ErrMessageBodyTooLong) {
		t.Fatalf("body over the default limit error = %v", err)
	}
}

func TestServiceAppliesInjectedLimitsOnEveryMessagePath(t *testing.T) {
	limits := raisedContentLimits()
	service := newLengthLimitService(t, &limits)

	// Longer than the domain default, still within the injected limits.
	withinInjectedTitle := strings.Repeat("标", domain.MaxMessageTitleRunes+1)
	withinInjectedBody := strings.Repeat("文", domain.MaxMessageBodyRunes+1)
	overInjectedTitle := strings.Repeat("标", raisedTitleRunes+1)
	overInjectedBody := strings.Repeat("文", raisedBodyRunes+1)

	for _, path := range messageLengthPaths() {
		t.Run(path.name, func(t *testing.T) {
			if err := path.apply(t, service, withinInjectedTitle, withinInjectedBody); err != nil {
				t.Fatalf("message within the injected limits was rejected: %v", err)
			}
			if err := path.apply(t, service, overInjectedTitle, "Body"); !errors.Is(err, domain.ErrMessageTitleTooLong) {
				t.Fatalf("title over the injected limit error = %v", err)
			}
			if err := path.apply(t, service, "标题", overInjectedBody); !errors.Is(err, domain.ErrMessageBodyTooLong) {
				t.Fatalf("body over the injected limit error = %v", err)
			}
		})
	}
}
