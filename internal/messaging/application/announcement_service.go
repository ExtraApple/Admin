package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"admin/internal/messaging/domain"
)

const PermissionAnnouncementManage = "admin.messages.announcement.manage"

type CreateAnnouncementRequest struct {
	ActorID      uint
	CategoryCode string
	Title        string
	Markdown     string
	Targets      []DynamicAudienceTarget
	AllUsers     bool
	PublishAt    *time.Time
	ExpiresAt    *time.Time
}

func (service *Service) CreateAnnouncement(ctx context.Context, request CreateAnnouncementRequest) ([]domain.Message, error) {
	if service == nil || service.messages == nil || service.categories == nil || service.authorization == nil || service.organizations == nil || service.transactions == nil {
		return nil, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionAnnouncementManage); err != nil {
		return nil, err
	}
	audiencesByOrganization, err := service.resolveBroadcastAudiences(ctx, request.ActorID, request.Targets, request.AllUsers)
	if err != nil {
		return nil, err
	}
	compiled, err := domain.CompileMessageContent(request.Title, request.Markdown)
	if err != nil {
		return nil, err
	}
	now := service.clock.Now().UTC()
	status, publishAt := announcementInitialStatus(request.PublishAt, now)
	expiresAt := copyUTC(request.ExpiresAt)
	logicalID := uuid.NewString()
	copies := make([]domain.Message, 0, len(audiencesByOrganization))
	err = service.transactions.Run(ctx, func(tx context.Context) error {
		for _, organizationID := range sortedAudienceOrganizationIDs(audiencesByOrganization) {
			category, err := service.categories.FindCategoryByCode(tx, organizationID, request.CategoryCode)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return ErrCategoryUnavailable
				}
				return err
			}
			if !category.Enabled {
				return ErrCategoryUnavailable
			}
			message := domain.Message{
				LogicalID:        logicalID,
				OrganizationID:   organizationID,
				SenderID:         request.ActorID,
				CategoryID:       category.ID,
				Kind:             domain.MessageKindAnnouncement,
				Status:           status,
				Title:            compiled.Title,
				BodyHTML:         compiled.HTML,
				PublishAt:        publishAt,
				ExpiresAt:        expiresAt,
				AggregateVersion: 1,
			}
			var event *domain.MessageEvent
			if status == domain.MessageStatusPublished {
				event = &domain.MessageEvent{EventID: uuid.NewString(), EventName: domain.EventNameMessagePublished, EventVersion: 1, OrganizationID: organizationID, OccurredAt: now, AggregateVersion: 1}
			}
			copy, err := service.messages.PersistMessage(tx, MessagePersistence{Message: message, Audiences: audiencesByOrganization[organizationID], Event: event})
			if err != nil {
				return err
			}
			copies = append(copies, copy)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return copies, nil
}

func (service *Service) PublishDueAnnouncements(ctx context.Context, now time.Time) (int, error) {
	return service.transitionDueAnnouncements(ctx, now, domain.MessageStatusScheduled, domain.MessageStatusPublished, domain.EventNameMessagePublished, true)
}

func (service *Service) ExpireDueAnnouncements(ctx context.Context, now time.Time) (int, error) {
	return service.transitionDueAnnouncements(ctx, now, domain.MessageStatusPublished, domain.MessageStatusExpired, domain.EventNameMessageExpired, false)
}

func (service *Service) transitionDueAnnouncements(ctx context.Context, now time.Time, from, to domain.MessageStatus, eventName domain.EventName, byPublishTime bool) (int, error) {
	if service == nil || service.messages == nil || service.organizations == nil || service.transactions == nil {
		return 0, ErrMessagingDependency
	}
	if now.IsZero() {
		now = service.clock.Now()
	}
	now = now.UTC()
	allOrganizations, ok := service.organizations.(AllOrganizationAudienceReader)
	if !ok {
		return 0, ErrMessagingDependency
	}
	organizationIDs, err := allOrganizations.AllOrganizationIDs(ctx)
	if err != nil {
		return 0, err
	}
	query := MessageListQuery{OrganizationIDs: organizationIDs, Kinds: []domain.MessageKind{domain.MessageKindAnnouncement}, Statuses: []domain.MessageStatus{from}}
	if byPublishTime {
		query.PublishAtOnOrBefore = &now
	} else {
		query.ExpiresAtOnOrBefore = &now
	}
	messages, _, err := service.messages.ListMessages(ctx, query)
	if err != nil {
		return 0, err
	}
	if len(messages) == 0 {
		return 0, nil
	}
	changed := 0
	err = service.transactions.Run(ctx, func(tx context.Context) error {
		for _, message := range messages {
			if _, err := domain.TransitionMessage(message.Kind, message.Status, to); err != nil {
				return err
			}
			message.Status = to
			message.AggregateVersion++
			event := domain.MessageEvent{EventID: uuid.NewString(), EventName: eventName, EventVersion: 1, MessageCopyID: message.ID, OrganizationID: message.OrganizationID, OccurredAt: now, AggregateVersion: message.AggregateVersion}
			if _, err := service.messages.ChangeMessage(tx, MessageChange{Message: message, Event: event}); err != nil {
				return err
			}
			changed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

func announcementInitialStatus(requested *time.Time, now time.Time) (domain.MessageStatus, *time.Time) {
	publishAt := copyUTC(requested)
	if publishAt == nil {
		return domain.MessageStatusDraft, nil
	}
	if publishAt.After(now) {
		return domain.MessageStatusScheduled, publishAt
	}
	return domain.MessageStatusPublished, publishAt
}

func copyUTC(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

type EditAnnouncementRequest struct {
	ActorID      uint
	MessageID    uint
	CategoryCode string
	Title        string
	Markdown     string
	Targets      []DynamicAudienceTarget
	AllUsers     bool
	ExpiresAt    *time.Time
}

func (service *Service) EditAnnouncement(ctx context.Context, request EditAnnouncementRequest) (domain.Message, error) {
	if service == nil || service.messages == nil || service.categories == nil || service.authorization == nil || service.organizations == nil || service.transactions == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionAnnouncementManage); err != nil {
		return domain.Message{}, err
	}
	if request.MessageID == 0 {
		return domain.Message{}, ErrNotFound
	}
	message, err := service.messages.FindMessage(ctx, request.MessageID)
	if err != nil {
		return domain.Message{}, err
	}
	if message.Kind != domain.MessageKindAnnouncement {
		return domain.Message{}, ErrNotFound
	}
	if err := service.requireManagedOrganization(ctx, request.ActorID, message.OrganizationID); err != nil {
		return domain.Message{}, err
	}
	now := service.clock.Now().UTC()
	if message.Status != domain.MessageStatusPublished || (message.ExpiresAt != nil && !message.ExpiresAt.After(now)) {
		return domain.Message{}, ErrMessageImmutable
	}
	compiled, err := domain.CompileMessageContent(request.Title, request.Markdown)
	if err != nil {
		return domain.Message{}, err
	}
	audiencesByOrganization, err := service.resolveBroadcastAudiences(ctx, request.ActorID, request.Targets, request.AllUsers)
	if err != nil {
		return domain.Message{}, err
	}
	replacementAudiences, ok := audiencesByOrganization[message.OrganizationID]
	if !ok || len(audiencesByOrganization) != 1 {
		return domain.Message{}, ErrOrganizationNotManaged
	}
	categoryID := message.CategoryID
	if request.CategoryCode != "" {
		category, err := service.categories.FindCategoryByCode(ctx, message.OrganizationID, request.CategoryCode)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return domain.Message{}, ErrCategoryUnavailable
			}
			return domain.Message{}, err
		}
		if !category.Enabled {
			return domain.Message{}, ErrCategoryUnavailable
		}
		categoryID = category.ID
	}
	message.Title = compiled.Title
	message.BodyHTML = compiled.HTML
	message.CategoryID = categoryID
	message.ExpiresAt = copyUTC(request.ExpiresAt)
	if message.ExpiresAt != nil && !message.ExpiresAt.After(now) {
		return domain.Message{}, ErrMessageImmutable
	}
	message.AggregateVersion++
	event := domain.MessageEvent{EventID: uuid.NewString(), EventName: domain.EventNameMessageEdited, EventVersion: 1, MessageCopyID: message.ID, OrganizationID: message.OrganizationID, OccurredAt: now, AggregateVersion: message.AggregateVersion}
	var changed domain.Message
	err = service.transactions.Run(ctx, func(tx context.Context) error {
		updated, err := service.messages.ChangeMessage(tx, MessageChange{Message: message, ReplaceAudiences: &replacementAudiences, Event: event})
		if err != nil {
			return err
		}
		changed = updated
		return nil
	})
	if err != nil {
		return domain.Message{}, err
	}
	return changed, nil
}
