package application

import (
	"context"

	"github.com/google/uuid"

	"admin/internal/messaging/domain"
)

const (
	PermissionOutboxReplay      = "admin.messages.outbox.replay"
	PermissionConsumerDLQManage = "admin.messages.dead-letter.manage"
)

type PublishAnnouncementRequest struct {
	ActorID   uint
	MessageID uint
	ImageIDs  []uint
}

func (service *Service) ListAnnouncements(ctx context.Context, request ManagedMessageListRequest) ([]domain.Message, int64, error) {
	if service == nil || service.messages == nil {
		return nil, 0, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionAnnouncementManage); err != nil {
		return nil, 0, err
	}
	if request.Offset < 0 || request.Limit < 0 {
		return nil, 0, ErrInboxRequestInvalid
	}
	organizationIDs, err := service.managedOrganizationIDs(ctx, request.ActorID)
	if err != nil {
		return nil, 0, err
	}
	return service.messages.ListMessages(ctx, MessageListQuery{OrganizationIDs: organizationIDs, Kinds: []domain.MessageKind{domain.MessageKindAnnouncement}, Statuses: append([]domain.MessageStatus(nil), request.Statuses...), CategoryID: request.CategoryID, Keyword: request.Keyword, Offset: request.Offset, Limit: request.Limit})
}

func (service *Service) GetAnnouncement(ctx context.Context, actorID, messageID uint) (domain.Message, error) {
	if service == nil || service.messages == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, actorID, PermissionAnnouncementManage); err != nil {
		return domain.Message{}, err
	}
	if messageID == 0 {
		return domain.Message{}, ErrNotFound
	}
	message, err := service.messages.FindMessage(ctx, messageID)
	if err != nil {
		return domain.Message{}, err
	}
	if message.Kind != domain.MessageKindAnnouncement {
		return domain.Message{}, ErrNotFound
	}
	if err := service.requireManagedOrganization(ctx, actorID, message.OrganizationID); err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func (service *Service) PublishAnnouncement(ctx context.Context, request PublishAnnouncementRequest) (domain.Message, error) {
	if service == nil || service.messages == nil || service.transactions == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if len(request.ImageIDs) > 0 && service.files == nil {
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
	if !domain.CanTransitionMessage(message.Kind, message.Status, domain.MessageStatusScheduled) && !domain.CanTransitionMessage(message.Kind, message.Status, domain.MessageStatusPublished) {
		return domain.Message{}, ErrMessageImmutable
	}
	now := service.clock.Now().UTC()
	publishAt := message.PublishAt
	if publishAt != nil {
		planned := publishAt.UTC()
		publishAt = &planned
	}
	if publishAt != nil && publishAt.After(now) {
		message.Status = domain.MessageStatusScheduled
		message.PublishAt = publishAt
	} else {
		message.Status = domain.MessageStatusPublished
		message.PublishAt = &now
	}
	if message.ExpiresAt != nil && !message.ExpiresAt.After(now) {
		return domain.Message{}, ErrMessageImmutable
	}
	message.AggregateVersion++
	event := domain.MessageEvent{}
	if message.Status == domain.MessageStatusPublished {
		event = domain.MessageEvent{EventID: newMessageEventID(), EventName: domain.EventNameMessagePublished, EventVersion: 1, MessageCopyID: message.ID, OrganizationID: message.OrganizationID, OccurredAt: now, AggregateVersion: message.AggregateVersion}
	}
	var published domain.Message
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		changed, err := service.messages.ChangeMessage(tx, MessageChange{Message: message, Event: event})
		if err != nil {
			return err
		}
		if len(request.ImageIDs) > 0 {
			if err := service.files.BindMessageImages(tx, MessageImageBinding{ActorID: request.ActorID, MessageLogicalID: changed.LogicalID, ImageIDs: append([]uint(nil), request.ImageIDs...)}); err != nil {
				return err
			}
		}
		published = changed
		return nil
	}); err != nil {
		return domain.Message{}, err
	}
	return published, nil
}

func newMessageEventID() string {
	return uuid.NewString()
}

type AdminOutboxListRequest struct {
	ActorID  uint
	Statuses []domain.OutboxStatus
	Offset   int
	Limit    int
}

func (service *Service) ListOutboxes(ctx context.Context, request AdminOutboxListRequest) ([]domain.MessageOutbox, int64, error) {
	if service == nil || service.outboxes == nil {
		return nil, 0, ErrMessagingDependency
	}
	if err := service.requireSuperAdmin(ctx, request.ActorID, PermissionOutboxReplay); err != nil {
		return nil, 0, err
	}
	if request.Offset < 0 || request.Limit < 0 {
		return nil, 0, ErrInboxRequestInvalid
	}
	statuses := append([]domain.OutboxStatus(nil), request.Statuses...)
	if len(statuses) == 0 {
		statuses = []domain.OutboxStatus{domain.OutboxStatusDead}
	}
	return service.outboxes.ListOutboxes(ctx, OutboxListQuery{Statuses: statuses, Offset: request.Offset, Limit: request.Limit})
}

func (service *Service) ReplayOutbox(ctx context.Context, actorID, outboxID uint) error {
	if service == nil || service.outboxes == nil {
		return ErrMessagingDependency
	}
	if err := service.requireSuperAdmin(ctx, actorID, PermissionOutboxReplay); err != nil {
		return err
	}
	if outboxID == 0 {
		return ErrNotFound
	}
	changed, err := service.outboxes.ReplayOutbox(ctx, outboxID)
	if err != nil {
		return err
	}
	if !changed {
		return ErrNotFound
	}
	return nil
}

type AdminConsumerDeadLetterListRequest struct {
	ActorID      uint
	Statuses     []domain.ConsumerDLQStatus
	ConsumerName string
	Offset       int
	Limit        int
}

func (service *Service) ListConsumerDeadLetters(ctx context.Context, request AdminConsumerDeadLetterListRequest) ([]ConsumerDeadLetter, int64, error) {
	if service == nil || service.consumerDeadLetters == nil {
		return nil, 0, ErrMessagingDependency
	}
	if err := service.requireSuperAdmin(ctx, request.ActorID, PermissionConsumerDLQManage); err != nil {
		return nil, 0, err
	}
	if request.Offset < 0 || request.Limit < 0 {
		return nil, 0, ErrInboxRequestInvalid
	}
	return service.consumerDeadLetters.ListConsumerDeadLetters(ctx, ConsumerDeadLetterListQuery{Statuses: append([]domain.ConsumerDLQStatus(nil), request.Statuses...), ConsumerName: request.ConsumerName, Offset: request.Offset, Limit: request.Limit})
}

func (service *Service) ReplayConsumerDeadLetter(ctx context.Context, actorID, deadLetterID uint) (ConsumerDeadLetter, error) {
	if service == nil || service.consumerDeadLetterReplay == nil {
		return ConsumerDeadLetter{}, ErrMessagingDependency
	}
	if err := service.requireSuperAdmin(ctx, actorID, PermissionConsumerDLQManage); err != nil {
		return ConsumerDeadLetter{}, err
	}
	if deadLetterID == 0 {
		return ConsumerDeadLetter{}, ErrNotFound
	}
	deadLetter, replayed, err := service.consumerDeadLetterReplay.Replay(ctx, deadLetterID)
	if err != nil {
		return ConsumerDeadLetter{}, err
	}
	if !replayed {
		return ConsumerDeadLetter{}, ErrStateConflict
	}
	return deadLetter, nil
}

func (service *Service) DiscardConsumerDeadLetter(ctx context.Context, actorID, deadLetterID uint) error {
	if service == nil || service.consumerDeadLetters == nil {
		return ErrMessagingDependency
	}
	if err := service.requireSuperAdmin(ctx, actorID, PermissionConsumerDLQManage); err != nil {
		return err
	}
	if deadLetterID == 0 {
		return ErrNotFound
	}
	discarded, err := service.consumerDeadLetters.DiscardConsumerDeadLetter(ctx, deadLetterID, service.clock.Now().UTC())
	if err != nil {
		return err
	}
	if !discarded {
		return ErrNotFound
	}
	return nil
}

func (service *Service) requireSuperAdmin(ctx context.Context, actorID uint, permission string) error {
	if err := service.requirePermission(ctx, actorID, permission); err != nil {
		return err
	}
	if service.authorization == nil {
		return ErrPermissionDenied
	}
	scope, err := service.authorization.OrganizationScope(ctx, actorID)
	if err != nil {
		return err
	}
	if !scope.All {
		return ErrPermissionDenied
	}
	return nil
}
