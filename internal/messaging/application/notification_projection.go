package application

import (
	"context"
	"io"
	"sort"
	"strings"

	htmlparser "golang.org/x/net/html"

	"admin/internal/messaging/domain"
)

// NotificationProjectionRequest is the event identity supplied by an
// in-process notification consumer. The consumer receives no delivery
// credential or email address from this projection.
type NotificationProjectionRequest struct {
	Event domain.MessageEvent
}

// NotificationProjectionReader is the stable seam for future in-process
// email and SMS consumers. It intentionally exposes a projection method only;
// it is not a provider or delivery interface.
type NotificationProjectionReader interface {
	ProjectNotificationRecipients(context.Context, NotificationProjectionRequest) ([]NotificationProjection, error)
}

var _ NotificationProjectionReader = (*Service)(nil)

// NotificationProjection is the complete message content an injected
// notification consumer may deliver. Identity owns address lookup; Messaging
// supplies only the current enabled recipient and rendered message content.
type NotificationProjection struct {
	UserID           uint
	DisplayName      string
	MessageCopyID    uint
	EventVersion     uint
	AggregateVersion uint64
	Title            string
	BodyHTML         string
	BodyText         string
}

// ProjectNotificationRecipients returns the current unread recipients for a
// notification lifecycle event. Dynamic audiences are resolved at projection
// time, so membership and role changes affect delayed, retried, and replayed
// deliveries. A user already marked read is never returned.
func (service *Service) ProjectNotificationRecipients(ctx context.Context, request NotificationProjectionRequest) ([]NotificationProjection, error) {
	if service == nil || service.messages == nil || service.inbox == nil || service.identity == nil || service.organizations == nil {
		return nil, ErrMessagingDependency
	}
	if err := domain.ValidateMessageEvent(request.Event); err != nil {
		return nil, err
	}
	message, err := service.messages.FindMessage(ctx, request.Event.MessageCopyID)
	if err != nil {
		return nil, err
	}
	if message.ID != request.Event.MessageCopyID || message.OrganizationID != request.Event.OrganizationID {
		return nil, ErrNotificationProjectionInvalid
	}
	if !notificationLifecycleAllowed(message.Kind, request.Event.EventName) {
		return []NotificationProjection{}, nil
	}
	if message.AggregateVersion != request.Event.AggregateVersion {
		return nil, ErrNotificationProjectionInvalid
	}
	if message.Status != domain.MessageStatusPublished || (message.PublishAt != nil && message.PublishAt.After(service.clock.Now())) || (message.ExpiresAt != nil && !message.ExpiresAt.After(service.clock.Now())) {
		return []NotificationProjection{}, nil
	}

	userIDs, roleIDs, err := service.notificationAudience(ctx, message, request.Event.EventName)
	if err != nil {
		return nil, err
	}
	projections := make([]NotificationProjection, 0, len(userIDs))
	for _, userID := range userIDs {
		user, err := service.identity.LookupUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		if user.ID == 0 || !user.Enabled {
			continue
		}
		memberships := []OrganizationMembership(nil)
		if message.Kind != domain.MessageKindPrivate {
			memberships, err = service.organizations.Memberships(ctx, userID)
			if err != nil {
				return nil, err
			}
		}
		inbox, visible, err := service.inbox.FindInboxMessage(ctx, InboxIdentity{UserID: userID, MessageID: message.ID, Memberships: memberships, RoleIDs: roleIDs, Now: service.clock.Now().UTC()})
		if err != nil {
			return nil, err
		}
		if !visible || inbox.PrivateDeleted || inbox.ReadAt != nil {
			continue
		}
		projections = append(projections, NotificationProjection{
			UserID:           userID,
			DisplayName:      user.DisplayName,
			MessageCopyID:    message.ID,
			EventVersion:     request.Event.EventVersion,
			AggregateVersion: message.AggregateVersion,
			Title:            message.Title,
			BodyHTML:         message.BodyHTML,
			BodyText:         notificationPlainText(message.BodyHTML),
		})
	}
	return projections, nil
}

func notificationLifecycleAllowed(kind domain.MessageKind, eventName domain.EventName) bool {
	switch {
	case kind == domain.MessageKindPrivate && eventName == domain.EventNameMessageCreated:
		return true
	case (kind == domain.MessageKindBroadcast || kind == domain.MessageKindAnnouncement) && eventName == domain.EventNameMessagePublished:
		return true
	default:
		return false
	}
}

func (service *Service) notificationAudience(ctx context.Context, message domain.Message, eventName domain.EventName) ([]uint, []uint, error) {
	if message.Kind == domain.MessageKindPrivate && eventName == domain.EventNameMessageCreated {
		if service.notifications == nil {
			return nil, nil, ErrMessagingDependency
		}
		userIDs, err := service.notifications.ListPrivateNotificationUserIDs(ctx, message.ID)
		return uniqueNotificationUserIDs(userIDs), nil, err
	}
	rules, err := service.messages.ListAudienceRules(ctx, message.ID)
	if err != nil {
		return nil, nil, err
	}
	userSet := make(map[uint]struct{})
	roleSet := make(map[uint]struct{})
	for _, rule := range rules {
		if err := domain.ValidateAudienceRule(rule); err != nil {
			return nil, nil, err
		}
		var users []uint
		switch rule.Type {
		case domain.AudienceTypeOrganization, domain.AudienceTypeAll:
			users, err = service.organizations.MemberUserIDs(ctx, rule.OrganizationID)
		case domain.AudienceTypeRole:
			users, err = service.organizations.RoleMemberUserIDs(ctx, rule.OrganizationID, rule.RoleID)
			roleSet[rule.RoleID] = struct{}{}
		}
		if err != nil {
			return nil, nil, err
		}
		for _, userID := range users {
			if userID != 0 {
				userSet[userID] = struct{}{}
			}
		}
	}
	userIDs := make([]uint, 0, len(userSet))
	for userID := range userSet {
		userIDs = append(userIDs, userID)
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })
	roleIDs := make([]uint, 0, len(roleSet))
	for roleID := range roleSet {
		roleIDs = append(roleIDs, roleID)
	}
	sort.Slice(roleIDs, func(i, j int) bool { return roleIDs[i] < roleIDs[j] })
	return userIDs, roleIDs, nil
}

func uniqueNotificationUserIDs(userIDs []uint) []uint {
	set := make(map[uint]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID != 0 {
			set[userID] = struct{}{}
		}
	}
	result := make([]uint, 0, len(set))
	for userID := range set {
		result = append(result, userID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func notificationPlainText(bodyHTML string) string {
	tokenizer := htmlparser.NewTokenizer(strings.NewReader(bodyHTML))
	var text strings.Builder
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case htmlparser.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return strings.Join(strings.Fields(text.String()), " ")
			}
			return strings.Join(strings.Fields(text.String()), " ")
		case htmlparser.TextToken:
			text.WriteString(string(tokenizer.Text()))
		case htmlparser.StartTagToken, htmlparser.EndTagToken, htmlparser.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			if notificationBlockTag(string(name)) {
				text.WriteByte('\n')
			}
		}
	}
}

func notificationBlockTag(name string) bool {
	switch strings.ToLower(name) {
	case "p", "br", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6", "li", "pre", "hr":
		return true
	default:
		return false
	}
}
