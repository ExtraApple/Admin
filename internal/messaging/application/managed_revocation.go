package application

import (
	"context"

	"github.com/google/uuid"

	"admin/internal/messaging/domain"
)

const PermissionMessageRevokeAll = "admin.messages.revoke.all"

type ManagedRevokeRequest struct {
	ActorID   uint
	MessageID uint
}

func (service *Service) RevokeManagedMessage(ctx context.Context, request ManagedRevokeRequest) (domain.Message, error) {
	if service == nil || service.messages == nil || service.authorization == nil || service.organizations == nil || service.transactions == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionMessageRevokeAll); err != nil {
		return domain.Message{}, err
	}
	if request.MessageID == 0 {
		return domain.Message{}, ErrRevokeNotAllowed
	}
	message, err := service.messages.FindMessage(ctx, request.MessageID)
	if err != nil {
		return domain.Message{}, err
	}
	actorScope, err := service.authorization.OrganizationScope(ctx, request.ActorID)
	if err != nil {
		return domain.Message{}, err
	}
	if !actorScope.All {
		managedOrganizationIDs, err := service.organizations.DescendantOrganizationIDs(ctx, actorScope.OrganizationIDs)
		if err != nil {
			return domain.Message{}, err
		}
		if !containsOrganizationID(managedOrganizationIDs, message.OrganizationID) {
			return domain.Message{}, ErrOrganizationNotManaged
		}
		if message.Kind != domain.MessageKindAnnouncement || message.SenderID != request.ActorID {
			senderScope, err := service.authorization.OrganizationScope(ctx, message.SenderID)
			if err != nil {
				return domain.Message{}, err
			}
			if senderScope.All || len(senderScope.OrganizationIDs) != 0 {
				return domain.Message{}, ErrRevokeNotAllowed
			}
		}
	}
	if message.Status != domain.MessageStatusPublished {
		return domain.Message{}, ErrRevokeNotAllowed
	}
	if _, err := domain.TransitionMessage(message.Kind, message.Status, domain.MessageStatusRevoked); err != nil {
		return domain.Message{}, ErrRevokeNotAllowed
	}
	now := service.clock.Now().UTC()
	message.Status = domain.MessageStatusRevoked
	message.RevokedAt = &now
	message.AggregateVersion++
	event := domain.MessageEvent{EventID: uuid.NewString(), EventName: domain.EventNameMessageRevoked, EventVersion: 1, MessageCopyID: message.ID, OrganizationID: message.OrganizationID, OccurredAt: now, AggregateVersion: message.AggregateVersion}
	var changed domain.Message
	err = service.transactions.Run(ctx, func(tx context.Context) error {
		updated, err := service.messages.ChangeMessage(tx, MessageChange{Message: message, Event: event})
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

func containsOrganizationID(ids []uint, expected uint) bool {
	for _, id := range ids {
		if id == expected {
			return true
		}
	}
	return false
}
