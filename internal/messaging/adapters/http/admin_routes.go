package httpadapter

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

func AdminRoutes(service *application.Service) []routecatalog.Descriptor {
	h := &adminHandler{service: service, user: &handler{service: service}}
	return []routecatalog.Descriptor{
		adminRoute(http.MethodGet, "/api/admin/messages", "List Broadcast Messages", application.PermissionBroadcastManage, h.listBroadcasts, nil, ManagedMessageListResponse{}),
		adminRoute(http.MethodPost, "/api/admin/messages/broadcast", "Create Broadcast Message", application.PermissionBroadcastManage, h.createBroadcast, CreateBroadcastRequest{}, []MessageDTO{}),
		adminRoute(http.MethodPost, "/api/admin/messages/:id/revoke", "Revoke Managed Message", application.PermissionMessageRevokeAll, h.revokeMessage, nil, MessageDTO{}),
		adminRoute(http.MethodGet, "/api/admin/announcements", "List Announcements", application.PermissionAnnouncementManage, h.listAnnouncements, nil, ManagedMessageListResponse{}),
		adminRoute(http.MethodPost, "/api/admin/announcements", "Create Announcement", application.PermissionAnnouncementManage, h.createAnnouncement, CreateAnnouncementRequest{}, []MessageDTO{}),
		adminRoute(http.MethodGet, "/api/admin/announcements/:id", "Get Announcement", application.PermissionAnnouncementManage, h.getAnnouncement, nil, MessageDTO{}),
		adminRoute(http.MethodPut, "/api/admin/announcements/:id", "Edit Announcement", application.PermissionAnnouncementManage, h.editAnnouncement, EditAnnouncementRequest{}, MessageDTO{}),
		adminRoute(http.MethodPost, "/api/admin/announcements/:id/publish", "Publish Announcement", application.PermissionAnnouncementManage, h.publishAnnouncement, PublishAnnouncementRequest{}, MessageDTO{}),
		adminRoute(http.MethodPost, "/api/admin/announcements/:id/revoke", "Revoke Announcement", application.PermissionMessageRevokeAll, h.revokeMessage, nil, MessageDTO{}),
		adminRoute(http.MethodGet, "/api/admin/message-categories", "List Message Categories", application.PermissionCategoryManage, h.listCategories, nil, CategoryListResponse{}),
		adminRoute(http.MethodPost, "/api/admin/message-categories", "Create Message Category", application.PermissionCategoryManage, h.createCategory, CreateMessageCategoryRequest{}, MessageCategoryDTO{}),
		adminRoute(http.MethodPut, "/api/admin/message-categories/:id", "Update Message Category", application.PermissionCategoryManage, h.updateCategory, UpdateMessageCategoryRequest{}, MessageCategoryDTO{}),
		adminRoute(http.MethodDelete, "/api/admin/message-categories/:id", "Delete Message Category", application.PermissionCategoryManage, h.deleteCategory, nil, nil),
		adminImageRoute(h.user.uploadImage),
		adminRoute(http.MethodGet, "/api/admin/message-outboxes", "List Message Outboxes", application.PermissionOutboxReplay, h.listOutboxes, nil, MessageOutboxListResponse{}),
		adminRoute(http.MethodPost, "/api/admin/message-outboxes/:id/replay", "Replay Message Outbox", application.PermissionOutboxReplay, h.replayOutbox, nil, nil),
		adminRoute(http.MethodGet, "/api/admin/message-dead-letters", "List Message Dead Letters", application.PermissionConsumerDLQManage, h.listDeadLetters, nil, ConsumerDeadLetterListResponse{}),
		adminRoute(http.MethodPost, "/api/admin/message-dead-letters/:id/replay", "Replay Message Dead Letter", application.PermissionConsumerDLQManage, h.replayDeadLetter, nil, ConsumerDeadLetterDTO{}),
		adminRoute(http.MethodDelete, "/api/admin/message-dead-letters/:id", "Discard Message Dead Letter", application.PermissionConsumerDLQManage, h.discardDeadLetter, nil, nil),
	}
}

func adminRoute(method, path, name, permission string, fn gin.HandlerFunc, request, response any) routecatalog.Descriptor {
	descriptor := messageRoute(method, path, name, routecatalog.PermissionControlled, fn, request, response)
	descriptor.DefaultPermissionCode = permission
	descriptor.OpenAPI.Description = "Messaging administrator operation protected by the declared permission code."
	return descriptor
}

func adminImageRoute(fn gin.HandlerFunc) routecatalog.Descriptor {
	descriptor := messageImageRoute(http.MethodPost, "/api/admin/messages/images", "Upload Admin Message Image", routecatalog.PermissionControlled, fn)
	descriptor.DefaultPermissionCode = application.PermissionAnnouncementManage
	descriptor.OpenAPI.Description = "Upload a validated temporary message image for an administrator message operation."
	return descriptor
}

type adminHandler struct {
	service *application.Service
	user    *handler
}

func (h *adminHandler) listBroadcasts(c *gin.Context) {
	request, err := parseManagedMessageList(c, c.GetUint("userID"))
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messages, total, err := h.service.ListBroadcasts(c.Request.Context(), request)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, ManagedMessageListResponse{Items: toDomainMessageDTOs(messages), Total: total, Offset: request.Offset, Limit: request.Limit})
}

func (h *adminHandler) createBroadcast(c *gin.Context) {
	var request CreateBroadcastRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	messages, err := h.service.CreateBroadcast(c.Request.Context(), application.CreateBroadcastRequest{ActorID: c.GetUint("userID"), CategoryCode: request.CategoryCode, Title: request.Title, Markdown: request.Markdown, Targets: dynamicAudienceTargets(request.Targets), AllUsers: request.AllUsers, ImageIDs: append([]uint(nil), request.ImageIDs...)})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toDomainMessageDTOs(messages))
}

func (h *adminHandler) listAnnouncements(c *gin.Context) {
	request, err := parseManagedMessageList(c, c.GetUint("userID"))
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messages, total, err := h.service.ListAnnouncements(c.Request.Context(), request)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, ManagedMessageListResponse{Items: toDomainMessageDTOs(messages), Total: total, Offset: request.Offset, Limit: request.Limit})
}

func (h *adminHandler) createAnnouncement(c *gin.Context) {
	var request CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	messages, err := h.service.CreateAnnouncement(c.Request.Context(), application.CreateAnnouncementRequest{ActorID: c.GetUint("userID"), CategoryCode: request.CategoryCode, Title: request.Title, Markdown: request.Markdown, Targets: dynamicAudienceTargets(request.Targets), AllUsers: request.AllUsers, PublishAt: request.PublishAt, ExpiresAt: request.ExpiresAt, ImageIDs: append([]uint(nil), request.ImageIDs...)})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toDomainMessageDTOs(messages))
}

func (h *adminHandler) getAnnouncement(c *gin.Context) {
	messageID, ok := messagePathID(c)
	if !ok {
		return
	}
	message, err := h.service.GetAnnouncement(c.Request.Context(), c.GetUint("userID"), messageID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toMessageDTO(application.InboxMessage{Message: message}))
}

func (h *adminHandler) editAnnouncement(c *gin.Context) {
	messageID, ok := messagePathID(c)
	if !ok {
		return
	}
	var request EditAnnouncementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	message, err := h.service.EditAnnouncement(c.Request.Context(), application.EditAnnouncementRequest{ActorID: c.GetUint("userID"), MessageID: messageID, CategoryCode: request.CategoryCode, Title: request.Title, Markdown: request.Markdown, Targets: dynamicAudienceTargets(request.Targets), AllUsers: request.AllUsers, ExpiresAt: request.ExpiresAt, ImageIDs: append([]uint(nil), request.ImageIDs...)})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toMessageDTO(application.InboxMessage{Message: message}))
}

func (h *adminHandler) publishAnnouncement(c *gin.Context) {
	messageID, ok := messagePathID(c)
	if !ok {
		return
	}
	var request PublishAnnouncementRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	message, err := h.service.PublishAnnouncement(c.Request.Context(), application.PublishAnnouncementRequest{ActorID: c.GetUint("userID"), MessageID: messageID, ImageIDs: append([]uint(nil), request.ImageIDs...)})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toMessageDTO(application.InboxMessage{Message: message}))
}

func (h *adminHandler) revokeMessage(c *gin.Context) {
	messageID, ok := messagePathID(c)
	if !ok {
		return
	}
	message, err := h.service.RevokeManagedMessage(c.Request.Context(), application.ManagedRevokeRequest{ActorID: c.GetUint("userID"), MessageID: messageID})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toMessageDTO(application.InboxMessage{Message: message}))
}

func (h *adminHandler) listCategories(c *gin.Context) {
	page, size, err := parsePage(c)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	enabledOnly, err := optionalBoolQuery(c, "enabled_only")
	if err != nil {
		writeMessageError(c, err)
		return
	}
	categories, total, err := h.service.ListCategories(c.Request.Context(), application.CategoryListRequest{ActorID: c.GetUint("userID"), EnabledOnly: enabledOnly != nil && *enabledOnly, Offset: (page - 1) * size, Limit: size})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	result := make([]MessageCategoryDTO, len(categories))
	for index, category := range categories {
		result[index] = toCategoryDTO(category)
	}
	messageSuccess(c, CategoryListResponse{Items: result, Total: total})
}

func (h *adminHandler) createCategory(c *gin.Context) {
	var request CreateMessageCategoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	category, err := h.service.CreateCategory(c.Request.Context(), application.CategoryCreateRequest{ActorID: c.GetUint("userID"), OrganizationID: request.OrganizationID, Code: request.Code, Name: request.Name, Sort: request.Sort, Enabled: request.Enabled})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toCategoryDTO(category))
}

func (h *adminHandler) updateCategory(c *gin.Context) {
	categoryID, ok := messagePathID(c)
	if !ok {
		return
	}
	var request UpdateMessageCategoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	category, err := h.service.UpdateCategory(c.Request.Context(), application.CategoryUpdateRequest{ActorID: c.GetUint("userID"), CategoryID: categoryID, Name: request.Name, Sort: request.Sort, Enabled: request.Enabled})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toCategoryDTO(category))
}

func (h *adminHandler) deleteCategory(c *gin.Context) {
	categoryID, ok := messagePathID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteCategory(c.Request.Context(), application.CategoryDeleteRequest{ActorID: c.GetUint("userID"), CategoryID: categoryID}); err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, nil)
}

func (h *adminHandler) listOutboxes(c *gin.Context) {
	page, size, err := parsePage(c)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	statuses, err := outboxStatusesQuery(c)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	items, total, err := h.service.ListOutboxes(c.Request.Context(), application.AdminOutboxListRequest{ActorID: c.GetUint("userID"), Statuses: statuses, Offset: (page - 1) * size, Limit: size})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	result := make([]MessageOutboxDTO, len(items))
	for index, item := range items {
		result[index] = toOutboxDTO(item)
	}
	messageSuccess(c, MessageOutboxListResponse{Items: result, Total: total, Offset: (page - 1) * size, Limit: size})
}

func (h *adminHandler) replayOutbox(c *gin.Context) {
	id, ok := messagePathID(c)
	if !ok {
		return
	}
	if err := h.service.ReplayOutbox(c.Request.Context(), c.GetUint("userID"), id); err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, nil)
}

func (h *adminHandler) listDeadLetters(c *gin.Context) {
	page, size, err := parsePage(c)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	statuses, err := deadLetterStatusesQuery(c)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	items, total, err := h.service.ListConsumerDeadLetters(c.Request.Context(), application.AdminConsumerDeadLetterListRequest{ActorID: c.GetUint("userID"), Statuses: statuses, ConsumerName: c.Query("consumer"), Offset: (page - 1) * size, Limit: size})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	result := make([]ConsumerDeadLetterDTO, len(items))
	for index, item := range items {
		result[index] = toDeadLetterDTO(item)
	}
	messageSuccess(c, ConsumerDeadLetterListResponse{Items: result, Total: total, Offset: (page - 1) * size, Limit: size})
}

func (h *adminHandler) replayDeadLetter(c *gin.Context) {
	id, ok := messagePathID(c)
	if !ok {
		return
	}
	item, err := h.service.ReplayConsumerDeadLetter(c.Request.Context(), c.GetUint("userID"), id)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toDeadLetterDTO(item))
}

func (h *adminHandler) discardDeadLetter(c *gin.Context) {
	id, ok := messagePathID(c)
	if !ok {
		return
	}
	if err := h.service.DiscardConsumerDeadLetter(c.Request.Context(), c.GetUint("userID"), id); err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, nil)
}

func parseManagedMessageList(c *gin.Context, actorID uint) (application.ManagedMessageListRequest, error) {
	page, size, err := parsePage(c)
	if err != nil {
		return application.ManagedMessageListRequest{}, err
	}
	statuses, err := messageStatusesQuery(c)
	if err != nil {
		return application.ManagedMessageListRequest{}, err
	}
	categoryID, err := optionalUintQuery(c, "category_id")
	if err != nil {
		return application.ManagedMessageListRequest{}, err
	}
	return application.ManagedMessageListRequest{ActorID: actorID, Statuses: statuses, CategoryID: categoryID, Keyword: c.Query("keyword"), Offset: (page - 1) * size, Limit: size}, nil
}

func parsePage(c *gin.Context) (int, int, error) {
	page, err := positiveQueryInt(c, "page", 1)
	if err != nil {
		return 0, 0, err
	}
	size, err := positiveQueryInt(c, "size", defaultMessagePageSize)
	if err != nil || size > maxMessagePageSize {
		return 0, 0, application.ErrInboxRequestInvalid
	}
	return page, size, nil
}

func messageStatusesQuery(c *gin.Context) ([]domain.MessageStatus, error) {
	values := c.QueryArray("status")
	result := make([]domain.MessageStatus, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			status := domain.MessageStatus(strings.TrimSpace(part))
			switch status {
			case domain.MessageStatusDraft, domain.MessageStatusScheduled, domain.MessageStatusPublished, domain.MessageStatusExpired, domain.MessageStatusRevoked:
				result = append(result, status)
			case "":
			default:
				return nil, application.ErrInboxRequestInvalid
			}
		}
	}
	return result, nil
}

func outboxStatusesQuery(c *gin.Context) ([]domain.OutboxStatus, error) {
	values := c.QueryArray("status")
	result := make([]domain.OutboxStatus, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			status := domain.OutboxStatus(strings.TrimSpace(part))
			switch status {
			case domain.OutboxStatusPending, domain.OutboxStatusPublishing, domain.OutboxStatusPublished, domain.OutboxStatusDead:
				result = append(result, status)
			case "":
			default:
				return nil, application.ErrInboxRequestInvalid
			}
		}
	}
	return result, nil
}

func deadLetterStatusesQuery(c *gin.Context) ([]domain.ConsumerDLQStatus, error) {
	values := c.QueryArray("status")
	result := make([]domain.ConsumerDLQStatus, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			status := domain.ConsumerDLQStatus(strings.TrimSpace(part))
			switch status {
			case domain.ConsumerDLQStatusPending, domain.ConsumerDLQStatusReplaying, domain.ConsumerDLQStatusReplayed, domain.ConsumerDLQStatusDiscarded:
				result = append(result, status)
			case "":
			default:
				return nil, application.ErrInboxRequestInvalid
			}
		}
	}
	return result, nil
}

func toDomainMessageDTOs(messages []domain.Message) []MessageDTO {
	result := make([]MessageDTO, len(messages))
	for index, message := range messages {
		result[index] = toMessageDTO(application.InboxMessage{Message: message})
	}
	return result
}

func toCategoryDTO(category domain.MessageCategory) MessageCategoryDTO {
	return MessageCategoryDTO{ID: category.ID, OrganizationID: category.OrganizationID, Code: category.Code, Name: category.Name, Sort: category.Sort, Enabled: category.Enabled}
}

func toOutboxDTO(outbox domain.MessageOutbox) MessageOutboxDTO {
	return MessageOutboxDTO{ID: outbox.ID, Event: outbox.Event, Status: string(outbox.Status), RetryAttempt: outbox.RetryAttempt, NextRetryAt: outbox.NextRetryAt, LastFailureCode: outbox.LastFailureCode}
}

func toDeadLetterDTO(deadLetter application.ConsumerDeadLetter) ConsumerDeadLetterDTO {
	return ConsumerDeadLetterDTO{ID: deadLetter.ID, ConsumerName: deadLetter.ConsumerName, EventID: deadLetter.Event.EventID, OriginalQueue: deadLetter.OriginalQueue, EventName: string(deadLetter.Event.EventName), EventVersion: deadLetter.Event.EventVersion, MessageCopyID: deadLetter.Event.MessageCopyID, OrganizationID: deadLetter.Event.OrganizationID, AggregateVersion: deadLetter.Event.AggregateVersion, RetryAttempt: deadLetter.RetryAttempt, Status: string(deadLetter.Status), ReplayCycle: deadLetter.ReplayCycle, LastFailureCode: deadLetter.LastFailureCode, AudienceObservedCount: deadLetter.AudienceObservedCount, Invalid: deadLetter.Invalid, Fingerprint: deadLetter.Fingerprint, Replayable: deadLetter.Replayable, FinalizedAt: deadLetter.FinalizedAt}
}
