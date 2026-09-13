package application

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"

	"admin/internal/messaging/domain"
)

const PermissionBroadcastManage = "admin.messages.broadcast.manage"

type DynamicAudienceTarget struct {
	OrganizationID uint
	Type           domain.AudienceType
	RoleID         uint
}

type CreateBroadcastRequest struct {
	ActorID      uint
	CategoryCode string
	Title        string
	Markdown     string
	Targets      []DynamicAudienceTarget
	AllUsers     bool
	ImageIDs     []uint
}

func (service *Service) CreateBroadcast(ctx context.Context, request CreateBroadcastRequest) ([]domain.Message, error) {
	if service == nil || service.messages == nil || service.categories == nil || service.authorization == nil || service.organizations == nil || service.transactions == nil {
		return nil, ErrMessagingDependency
	}
	if len(request.ImageIDs) > 0 && service.files == nil {
		return nil, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionBroadcastManage); err != nil {
		return nil, err
	}
	audiencesByOrganization, err := service.resolveBroadcastAudiences(ctx, request.ActorID, request.Targets, request.AllUsers)
	if err != nil {
		return nil, err
	}
	compiled, err := domain.CompileMessageContent(request.Title, request.Markdown, service.contentLimits)
	if err != nil {
		return nil, err
	}
	now := service.clock.Now().UTC()
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
				Kind:             domain.MessageKindBroadcast,
				Status:           domain.MessageStatusPublished,
				Title:            compiled.Title,
				BodyHTML:         compiled.HTML,
				PublishAt:        &now,
				AggregateVersion: 1,
			}
			event := domain.MessageEvent{
				EventID:          uuid.NewString(),
				EventName:        domain.EventNameMessagePublished,
				EventVersion:     1,
				OrganizationID:   organizationID,
				OccurredAt:       now,
				AggregateVersion: 1,
			}
			copy, err := service.messages.PersistMessage(tx, MessagePersistence{Message: message, Audiences: audiencesByOrganization[organizationID], Event: &event})
			if err != nil {
				return err
			}
			copies = append(copies, copy)
		}
		if len(request.ImageIDs) > 0 {
			if err := service.files.BindMessageImages(tx, MessageImageBinding{ActorID: request.ActorID, MessageLogicalID: logicalID, ImageIDs: append([]uint(nil), request.ImageIDs...)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return copies, nil
}

func (service *Service) resolveBroadcastAudiences(ctx context.Context, actorID uint, targets []DynamicAudienceTarget, allUsers bool) (map[uint][]domain.AudienceRule, error) {
	scope, err := service.authorization.OrganizationScope(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if allUsers {
		if len(targets) != 0 || !scope.All {
			return nil, ErrOrganizationNotManaged
		}
		allOrganizations, ok := service.organizations.(AllOrganizationAudienceReader)
		if !ok {
			return nil, ErrMessagingDependency
		}
		organizationIDs, err := allOrganizations.AllOrganizationIDs(ctx)
		if err != nil {
			return nil, err
		}
		if len(organizationIDs) == 0 {
			return nil, ErrBroadcastAudienceInvalid
		}
		result := make(map[uint][]domain.AudienceRule, len(organizationIDs))
		for _, organizationID := range uniqueOrganizationIDs(organizationIDs) {
			if organizationID == 0 {
				continue
			}
			result[organizationID] = []domain.AudienceRule{{OrganizationID: organizationID, Type: domain.AudienceTypeAll}}
		}
		if len(result) == 0 {
			return nil, ErrBroadcastAudienceInvalid
		}
		return result, nil
	}
	if len(targets) == 0 {
		return nil, ErrBroadcastAudienceInvalid
	}
	managed, err := service.managedOrganizationIDs(ctx, actorID)
	if err != nil {
		return nil, err
	}
	managedSet := make(map[uint]struct{}, len(managed))
	for _, organizationID := range managed {
		managedSet[organizationID] = struct{}{}
	}
	result := make(map[uint][]domain.AudienceRule, len(targets))
	seen := make(map[uint]map[audienceRuleKey]struct{}, len(targets))
	for _, target := range targets {
		if target.Type == domain.AudienceTypeAll {
			return nil, ErrBroadcastAudienceInvalid
		}
		rule := domain.AudienceRule{OrganizationID: target.OrganizationID, Type: target.Type, RoleID: target.RoleID}
		if err := domain.ValidateAudienceRule(rule); err != nil {
			return nil, ErrBroadcastAudienceInvalid
		}
		if _, ok := managedSet[rule.OrganizationID]; !ok {
			return nil, ErrOrganizationNotManaged
		}
		key := audienceRuleKey{typ: rule.Type, roleID: rule.RoleID}
		if seen[rule.OrganizationID] == nil {
			seen[rule.OrganizationID] = make(map[audienceRuleKey]struct{})
		}
		if _, duplicate := seen[rule.OrganizationID][key]; duplicate {
			continue
		}
		seen[rule.OrganizationID][key] = struct{}{}
		result[rule.OrganizationID] = append(result[rule.OrganizationID], rule)
	}
	return result, nil
}

type audienceRuleKey struct {
	typ    domain.AudienceType
	roleID uint
}

func sortedAudienceOrganizationIDs(audiences map[uint][]domain.AudienceRule) []uint {
	organizationIDs := make([]uint, 0, len(audiences))
	for organizationID := range audiences {
		organizationIDs = append(organizationIDs, organizationID)
	}
	sort.Slice(organizationIDs, func(left, right int) bool { return organizationIDs[left] < organizationIDs[right] })
	return organizationIDs
}

func uniqueOrganizationIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; id == 0 || duplicate {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

type ManagedMessageListRequest struct {
	ActorID    uint
	Statuses   []domain.MessageStatus
	CategoryID uint
	Keyword    string
	Offset     int
	Limit      int
}

type EditBroadcastRequest struct {
	ActorID   uint
	MessageID uint
	Title     string
	Markdown  string
}

func (service *Service) ListBroadcasts(ctx context.Context, request ManagedMessageListRequest) ([]domain.Message, int64, error) {
	if service == nil || service.messages == nil {
		return nil, 0, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionBroadcastManage); err != nil {
		return nil, 0, err
	}
	if request.Offset < 0 || request.Limit < 0 {
		return nil, 0, ErrInboxRequestInvalid
	}
	organizationIDs, err := service.managedOrganizationIDs(ctx, request.ActorID)
	if err != nil {
		return nil, 0, err
	}
	return service.messages.ListMessages(ctx, MessageListQuery{OrganizationIDs: organizationIDs, Kinds: []domain.MessageKind{domain.MessageKindBroadcast}, Statuses: append([]domain.MessageStatus(nil), request.Statuses...), CategoryID: request.CategoryID, Keyword: request.Keyword, Offset: request.Offset, Limit: request.Limit})
}

func (service *Service) EditBroadcast(ctx context.Context, request EditBroadcastRequest) (domain.Message, error) {
	if service == nil || service.messages == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if err := service.requirePermission(ctx, request.ActorID, PermissionBroadcastManage); err != nil {
		return domain.Message{}, err
	}
	if request.MessageID == 0 {
		return domain.Message{}, ErrNotFound
	}
	message, err := service.messages.FindMessage(ctx, request.MessageID)
	if err != nil {
		return domain.Message{}, err
	}
	if err := service.requireManagedOrganization(ctx, request.ActorID, message.OrganizationID); err != nil {
		return domain.Message{}, err
	}
	if message.Kind != domain.MessageKindBroadcast {
		return domain.Message{}, ErrNotFound
	}
	return domain.Message{}, ErrMessageImmutable
}
