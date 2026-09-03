package gormadapter

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"admin/internal/messaging/application"
	messagingdomain "admin/internal/messaging/domain"
	platformdatabase "admin/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository is the GORM adapter for the Messaging persistence interface.
// It maps only Messaging application values and never leaks its GORM records.
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ application.Repository = (*Repository)(nil)

func (repository *Repository) connection(ctx context.Context) *gorm.DB {
	return platformdatabase.FromContext(ctx, repository.db)
}

func (repository *Repository) CreateCategory(ctx context.Context, category messagingdomain.MessageCategory) (messagingdomain.MessageCategory, error) {
	if err := messagingdomain.ValidateMessageCategory(category); err != nil {
		return messagingdomain.MessageCategory{}, err
	}
	record := categoryFromDomain(category)
	if err := repository.connection(ctx).Create(&record).Error; err != nil {
		return messagingdomain.MessageCategory{}, err
	}
	return categoryToDomain(record), nil
}

func (repository *Repository) FindCategory(ctx context.Context, id uint) (messagingdomain.MessageCategory, error) {
	var record MessageCategory
	if err := repository.connection(ctx).First(&record, id).Error; err != nil {
		return messagingdomain.MessageCategory{}, mapRepositoryError(err)
	}
	return categoryToDomain(record), nil
}

func (repository *Repository) FindCategoryByCode(ctx context.Context, organizationID uint, code string) (messagingdomain.MessageCategory, error) {
	var record MessageCategory
	if err := repository.connection(ctx).Where("organization_id = ? AND code = ?", organizationID, code).First(&record).Error; err != nil {
		return messagingdomain.MessageCategory{}, mapRepositoryError(err)
	}
	return categoryToDomain(record), nil
}

func (repository *Repository) ListCategories(ctx context.Context, query application.CategoryListQuery) ([]messagingdomain.MessageCategory, int64, error) {
	db := repository.connection(ctx).Model(&MessageCategory{})
	if len(query.OrganizationIDs) == 0 {
		return []messagingdomain.MessageCategory{}, 0, nil
	}
	db = db.Where("organization_id IN ?", uniqueUintIDs(query.OrganizationIDs))
	if query.EnabledOnly {
		db = db.Where("enabled = ?", true)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []MessageCategory
	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	}
	if query.Offset > 0 {
		db = db.Offset(query.Offset)
	}
	if err := db.Order("organization_id asc, sort asc, id asc").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return categoriesToDomain(records), total, nil
}

func (repository *Repository) UpdateCategory(ctx context.Context, category messagingdomain.MessageCategory) error {
	if category.ID == 0 {
		return application.ErrNotFound
	}
	if err := messagingdomain.ValidateMessageCategory(category); err != nil {
		return err
	}
	result := repository.connection(ctx).Model(&MessageCategory{}).Where("id = ?", category.ID).Updates(map[string]any{
		"organization_id": category.OrganizationID,
		"code":            category.Code,
		"name":            category.Name,
		"sort":            category.Sort,
		"enabled":         category.Enabled,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (repository *Repository) DeleteCategoryIfUnused(ctx context.Context, id uint) (bool, error) {
	var used int64
	if err := repository.connection(ctx).Model(&Message{}).Where("category_id = ?", id).Count(&used).Error; err != nil {
		return false, err
	}
	if used != 0 {
		return false, nil
	}
	result := repository.connection(ctx).Delete(&MessageCategory{}, id)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected != 0, nil
}

func (repository *Repository) PersistMessage(ctx context.Context, persistence application.MessagePersistence) (messagingdomain.Message, error) {
	message := persistence.Message
	if message.AggregateVersion == 0 {
		message.AggregateVersion = 1
	}
	if err := messagingdomain.ValidateMessage(message); err != nil {
		return messagingdomain.Message{}, err
	}
	if persistence.Recipient != nil && (persistence.Recipient.SenderID == 0 || persistence.Recipient.RecipientID == 0 || persistence.Recipient.SenderID == persistence.Recipient.RecipientID) {
		return messagingdomain.Message{}, messagingdomain.ErrPrivateRecipientInvalid
	}
	for _, audience := range persistence.Audiences {
		if err := messagingdomain.ValidateAudienceRule(audience); err != nil {
			return messagingdomain.Message{}, err
		}
	}
	var persisted messagingdomain.Message
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		record := messageFromDomain(message)
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		persisted = messageToDomain(record)
		if err := createAudienceRecords(tx, record.ID, persistence.Audiences); err != nil {
			return err
		}
		if persistence.Recipient != nil {
			recipient := *persistence.Recipient
			if recipient.MessageID != 0 && recipient.MessageID != record.ID {
				return application.ErrStateConflict
			}
			recipient.MessageID = record.ID
			if err := messagingdomain.ValidatePrivateRecipient(recipient); err != nil {
				return err
			}
			if err := tx.Create(&MessageRecipient{MessageID: recipient.MessageID, SenderID: recipient.SenderID, RecipientID: recipient.RecipientID, ReadAt: recipient.ReadAt, DeletedAt: recipient.DeletedAt}).Error; err != nil {
				return err
			}
		}
		if persistence.Event == nil {
			return nil
		}
		event := *persistence.Event
		if event.MessageCopyID != 0 && event.MessageCopyID != record.ID {
			return application.ErrStateConflict
		}
		event.MessageCopyID = record.ID
		if event.OrganizationID != record.OrganizationID || event.AggregateVersion != record.AggregateVersion {
			return application.ErrStateConflict
		}
		if err := messagingdomain.ValidateMessageEvent(event); err != nil {
			return err
		}
		return tx.Create(&MessageOutbox{EventID: event.EventID, EventName: event.EventName, EventVersion: event.EventVersion, MessageCopyID: event.MessageCopyID, OrganizationID: event.OrganizationID, AggregateVersion: event.AggregateVersion, OccurredAt: event.OccurredAt, Status: messagingdomain.OutboxStatusPending}).Error
	})
	if err != nil {
		return messagingdomain.Message{}, err
	}
	return persisted, nil
}

func (repository *Repository) FindMessage(ctx context.Context, id uint) (messagingdomain.Message, error) {
	var record Message
	if err := repository.connection(ctx).First(&record, id).Error; err != nil {
		return messagingdomain.Message{}, mapRepositoryError(err)
	}
	return messageToDomain(record), nil
}

func (repository *Repository) ListMessages(ctx context.Context, query application.MessageListQuery) ([]messagingdomain.Message, int64, error) {
	db := repository.connection(ctx).Model(&Message{})
	if len(query.OrganizationIDs) == 0 {
		return []messagingdomain.Message{}, 0, nil
	}
	db = db.Where("organization_id IN ?", uniqueUintIDs(query.OrganizationIDs))
	if len(query.Kinds) > 0 {
		db = db.Where("kind IN ?", query.Kinds)
	}
	if len(query.Statuses) > 0 {
		db = db.Where("status IN ?", query.Statuses)
	}
	if query.PublishAtOnOrBefore != nil {
		db = db.Where("publish_at IS NOT NULL AND publish_at <= ?", query.PublishAtOnOrBefore.UTC())
	}
	if query.ExpiresAtOnOrBefore != nil {
		db = db.Where("expires_at IS NOT NULL AND expires_at <= ?", query.ExpiresAtOnOrBefore.UTC())
	}
	if query.CategoryID != 0 {
		db = db.Where("category_id = ?", query.CategoryID)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		db = db.Where("title LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []Message
	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	}
	if query.Offset > 0 {
		db = db.Offset(query.Offset)
	}
	if err := db.Order("publish_at desc, id desc").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return messagesToDomain(records), total, nil
}

func (repository *Repository) ChangeMessage(ctx context.Context, change application.MessageChange) (messagingdomain.Message, error) {
	message := change.Message
	if message.ID == 0 || message.AggregateVersion < 2 {
		return messagingdomain.Message{}, application.ErrStateConflict
	}
	if err := messagingdomain.ValidateMessage(message); err != nil {
		return messagingdomain.Message{}, err
	}
	event := change.Event
	if event.EventID == "" {
		if message.Kind != messagingdomain.MessageKindAnnouncement || message.Status != messagingdomain.MessageStatusScheduled {
			return messagingdomain.Message{}, application.ErrStateConflict
		}
	} else {
		if event.MessageCopyID != message.ID || event.OrganizationID != message.OrganizationID || event.AggregateVersion != message.AggregateVersion {
			return messagingdomain.Message{}, application.ErrStateConflict
		}
		if err := messagingdomain.ValidateMessageEvent(event); err != nil {
			return messagingdomain.Message{}, err
		}
	}
	if change.ReplaceAudiences != nil {
		for _, audience := range *change.ReplaceAudiences {
			if err := messagingdomain.ValidateAudienceRule(audience); err != nil {
				return messagingdomain.Message{}, err
			}
		}
	}
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Message{}).Where("id = ? AND aggregate_version = ?", message.ID, message.AggregateVersion-1).Updates(map[string]any{
			"organization_id":   message.OrganizationID,
			"sender_id":         message.SenderID,
			"category_id":       message.CategoryID,
			"kind":              message.Kind,
			"status":            message.Status,
			"title":             message.Title,
			"body_html":         message.BodyHTML,
			"publish_at":        message.PublishAt,
			"expires_at":        message.ExpiresAt,
			"revoked_at":        message.RevokedAt,
			"aggregate_version": message.AggregateVersion,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return application.ErrStateConflict
		}
		if change.ReplaceAudiences != nil {
			if err := tx.Where("message_id = ?", message.ID).Delete(&MessageAudience{}).Error; err != nil {
				return err
			}
			if err := createAudienceRecords(tx, message.ID, *change.ReplaceAudiences); err != nil {
				return err
			}
		}
		if event.EventID == "" {
			return nil
		}
		return tx.Create(&MessageOutbox{EventID: event.EventID, EventName: event.EventName, EventVersion: event.EventVersion, MessageCopyID: event.MessageCopyID, OrganizationID: event.OrganizationID, AggregateVersion: event.AggregateVersion, OccurredAt: event.OccurredAt, Status: messagingdomain.OutboxStatusPending}).Error
	})
	if err != nil {
		return messagingdomain.Message{}, err
	}
	return message, nil
}

func (repository *Repository) ListAudienceRules(ctx context.Context, messageID uint) ([]messagingdomain.AudienceRule, error) {
	var records []MessageAudience
	if err := repository.connection(ctx).Where("message_id = ?", messageID).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}
	rules := make([]messagingdomain.AudienceRule, len(records))
	for index, record := range records {
		rules[index] = messagingdomain.AudienceRule{Type: record.Type, OrganizationID: record.OrganizationID, RoleID: record.RoleID}
	}
	return rules, nil
}

func (repository *Repository) ListPrivateNotificationUserIDs(ctx context.Context, messageID uint) ([]uint, error) {
	if messageID == 0 {
		return nil, application.ErrNotificationProjectionInvalid
	}
	var userIDs []uint
	err := repository.connection(ctx).Model(&MessageRecipient{}).
		Where("message_id = ? AND deleted_at IS NULL", messageID).
		Order("recipient_id asc").Pluck("recipient_id", &userIDs).Error
	return userIDs, err
}

func createAudienceRecords(db *gorm.DB, messageID uint, audiences []messagingdomain.AudienceRule) error {
	if len(audiences) == 0 {
		return nil
	}
	records := make([]MessageAudience, len(audiences))
	for index, audience := range audiences {
		records[index] = MessageAudience{MessageID: messageID, OrganizationID: audience.OrganizationID, Type: audience.Type, RoleID: audience.RoleID}
	}
	return db.Create(&records).Error
}

func categoryToDomain(record MessageCategory) messagingdomain.MessageCategory {
	return messagingdomain.MessageCategory{ID: record.ID, OrganizationID: record.OrganizationID, Code: record.Code, Name: record.Name, Sort: record.Sort, Enabled: record.Enabled}
}

func categoryFromDomain(category messagingdomain.MessageCategory) MessageCategory {
	return MessageCategory{ID: category.ID, OrganizationID: category.OrganizationID, Code: category.Code, Name: category.Name, Sort: category.Sort, Enabled: category.Enabled}
}

func categoriesToDomain(records []MessageCategory) []messagingdomain.MessageCategory {
	categories := make([]messagingdomain.MessageCategory, len(records))
	for index, record := range records {
		categories[index] = categoryToDomain(record)
	}
	return categories
}

func messageToDomain(record Message) messagingdomain.Message {
	return messagingdomain.Message{ID: record.ID, LogicalID: record.LogicalID, OrganizationID: record.OrganizationID, SenderID: record.SenderID, CategoryID: record.CategoryID, Kind: record.Kind, Status: record.Status, Title: record.Title, BodyHTML: record.BodyHTML, PublishAt: record.PublishAt, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, AggregateVersion: record.AggregateVersion}
}

func messageFromDomain(message messagingdomain.Message) Message {
	return Message{ID: message.ID, LogicalID: message.LogicalID, OrganizationID: message.OrganizationID, SenderID: message.SenderID, CategoryID: message.CategoryID, Kind: message.Kind, Status: message.Status, Title: message.Title, BodyHTML: message.BodyHTML, PublishAt: message.PublishAt, ExpiresAt: message.ExpiresAt, RevokedAt: message.RevokedAt, AggregateVersion: message.AggregateVersion}
}

func messagesToDomain(records []Message) []messagingdomain.Message {
	messages := make([]messagingdomain.Message, len(records))
	for index, record := range records {
		messages[index] = messageToDomain(record)
	}
	return messages
}

func uniqueUintIDs(ids []uint) []uint {
	unique := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func mapRepositoryError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return application.ErrNotFound
	}
	return err
}

func (repository *Repository) ListInbox(ctx context.Context, query application.InboxQuery) ([]application.InboxMessage, int64, error) {
	if query.UserID == 0 {
		return []application.InboxMessage{}, 0, nil
	}
	query.Now = inboxNow(query.Now)
	records, err := repository.visibleInboxMessages(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	messageIDs := messageIDs(records)
	privateStates, dynamicStates, err := repository.inboxStates(ctx, query.UserID, messageIDs)
	if err != nil {
		return nil, 0, err
	}
	if err := repository.initializeDynamicInboxStates(ctx, query, records, dynamicStates); err != nil {
		return nil, 0, err
	}
	_, dynamicStates, err = repository.inboxStates(ctx, query.UserID, messageIDs)
	if err != nil {
		return nil, 0, err
	}
	inbox := make([]application.InboxMessage, 0, len(records))
	for _, record := range records {
		readAt := privateStates[record.ID]
		if record.Kind != messagingdomain.MessageKindPrivate {
			readAt = dynamicStates[record.ID]
		}
		if query.Read != nil && (*query.Read != (readAt != nil)) {
			continue
		}
		message := messageToDomain(record)
		if message.Status != messagingdomain.MessageStatusPublished {
			message.BodyHTML = ""
		}
		inbox = append(inbox, application.InboxMessage{Message: message, ReadAt: readAt})
	}
	total := int64(len(inbox))
	if query.Offset >= len(inbox) {
		return []application.InboxMessage{}, total, nil
	}
	start := query.Offset
	if start < 0 {
		start = 0
	}
	end := len(inbox)
	if query.Limit > 0 && start+query.Limit < end {
		end = start + query.Limit
	}
	return inbox[start:end], total, nil
}

func (repository *Repository) FindInboxMessage(ctx context.Context, identity application.InboxIdentity) (application.InboxMessage, bool, error) {
	if identity.MessageID == 0 || identity.UserID == 0 {
		return application.InboxMessage{}, false, nil
	}
	list, _, err := repository.ListInbox(ctx, application.InboxQuery{UserID: identity.UserID, Memberships: identity.Memberships, RoleIDs: identity.RoleIDs, Now: identity.Now})
	if err != nil {
		return application.InboxMessage{}, false, err
	}
	for _, message := range list {
		if message.Message.ID == identity.MessageID {
			return message, true, nil
		}
	}
	return application.InboxMessage{}, false, nil
}

func (repository *Repository) MarkInboxRead(ctx context.Context, identity application.InboxIdentity, readAt time.Time) (bool, error) {
	message, visible, err := repository.FindInboxMessage(ctx, identity)
	if err != nil || !visible {
		return false, err
	}
	if readAt.IsZero() {
		readAt = inboxNow(identity.Now)
	}
	if message.Message.Kind == messagingdomain.MessageKindPrivate {
		result := repository.connection(ctx).Model(&MessageRecipient{}).
			Where("message_id = ? AND recipient_id = ? AND deleted_at IS NULL AND read_at IS NULL", identity.MessageID, identity.UserID).
			Update("read_at", readAt)
		if result.Error != nil {
			return false, result.Error
		}
		return true, nil
	}
	result := repository.connection(ctx).Model(&MessageUserState{}).
		Where("message_id = ? AND user_id = ? AND read_at IS NULL", identity.MessageID, identity.UserID).
		Update("read_at", readAt)
	if result.Error != nil {
		return false, result.Error
	}
	return true, nil
}

func (repository *Repository) DeletePrivateInbox(ctx context.Context, identity application.InboxIdentity, deletedAt time.Time) (bool, error) {
	message, visible, err := repository.FindInboxMessage(ctx, identity)
	if err != nil || !visible || message.Message.Kind != messagingdomain.MessageKindPrivate {
		return false, err
	}
	if deletedAt.IsZero() {
		deletedAt = inboxNow(identity.Now)
	}
	result := repository.connection(ctx).Model(&MessageRecipient{}).
		Where("message_id = ? AND recipient_id = ? AND deleted_at IS NULL", identity.MessageID, identity.UserID).
		Update("deleted_at", deletedAt)
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) CountUnreadInbox(ctx context.Context, query application.InboxQuery) (application.UnreadInboxCount, error) {
	unread := false
	query.Read = &unread
	query.Offset, query.Limit = 0, 0
	list, _, err := repository.ListInbox(ctx, query)
	if err != nil {
		return application.UnreadInboxCount{}, err
	}
	counts := application.UnreadInboxCount{}
	for _, message := range list {
		switch message.Message.Kind {
		case messagingdomain.MessageKindPrivate:
			counts.Private++
		case messagingdomain.MessageKindBroadcast:
			counts.Broadcast++
		case messagingdomain.MessageKindAnnouncement:
			counts.Announcement++
		}
	}
	counts.Total = counts.Private + counts.Broadcast + counts.Announcement
	return counts, nil
}

func (repository *Repository) visibleInboxMessages(ctx context.Context, query application.InboxQuery) ([]Message, error) {
	db := repository.connection(ctx).Model(&Message{})
	privateClause := "(kind = ? AND EXISTS (SELECT 1 FROM message_recipients WHERE message_id = messages.id AND recipient_id = ? AND deleted_at IS NULL))"
	args := []any{messagingdomain.MessageKindPrivate, query.UserID}
	membershipIDs := membershipOrganizationIDs(query.Memberships)
	if len(membershipIDs) > 0 {
		dynamicClause, dynamicArgs := dynamicAudienceClause(membershipIDs, uniqueUintIDs(query.RoleIDs), query.Now)
		privateClause += " OR " + dynamicClause
		args = append(args, dynamicArgs...)
	}
	db = db.Where(privateClause, args...)
	if len(query.Kinds) > 0 {
		db = db.Where("kind IN ?", query.Kinds)
	}
	if query.CategoryID != 0 {
		db = db.Where("category_id = ?", query.CategoryID)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		db = db.Where("title LIKE ?", "%"+keyword+"%")
	}
	var records []Message
	if err := db.Order("publish_at desc, id desc").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func dynamicAudienceClause(organizationIDs, roleIDs []uint, now time.Time) (string, []any) {
	match := "audience.type IN ?"
	args := []any{[]messagingdomain.AudienceType{messagingdomain.AudienceTypeOrganization, messagingdomain.AudienceTypeAll}}
	if len(roleIDs) > 0 {
		match += " OR (audience.type = ? AND audience.role_id IN ?)"
		args = append(args, messagingdomain.AudienceTypeRole, roleIDs)
	}
	clause := "(kind IN ? AND status IN ? AND (publish_at IS NULL OR publish_at <= ?) AND (expires_at IS NULL OR expires_at > ?) AND EXISTS (SELECT 1 FROM message_audiences AS audience WHERE audience.message_id = messages.id AND audience.organization_id IN ? AND (" + match + ")))"
	args = append([]any{[]messagingdomain.MessageKind{messagingdomain.MessageKindBroadcast, messagingdomain.MessageKindAnnouncement}, []messagingdomain.MessageStatus{messagingdomain.MessageStatusPublished, messagingdomain.MessageStatusRevoked}, now, now, organizationIDs}, args...)
	return clause, args
}

func (repository *Repository) inboxStates(ctx context.Context, userID uint, messageIDs []uint) (map[uint]*time.Time, map[uint]*time.Time, error) {
	privateStates := make(map[uint]*time.Time)
	dynamicStates := make(map[uint]*time.Time)
	if len(messageIDs) == 0 {
		return privateStates, dynamicStates, nil
	}
	var recipients []MessageRecipient
	if err := repository.connection(ctx).Where("recipient_id = ? AND message_id IN ?", userID, messageIDs).Find(&recipients).Error; err != nil {
		return nil, nil, err
	}
	for _, recipient := range recipients {
		privateStates[recipient.MessageID] = recipient.ReadAt
	}
	var states []MessageUserState
	if err := repository.connection(ctx).Where("user_id = ? AND message_id IN ?", userID, messageIDs).Find(&states).Error; err != nil {
		return nil, nil, err
	}
	for _, state := range states {
		dynamicStates[state.MessageID] = state.ReadAt
	}
	return privateStates, dynamicStates, nil
}

func (repository *Repository) initializeDynamicInboxStates(ctx context.Context, query application.InboxQuery, records []Message, states map[uint]*time.Time) error {
	joinedAt := make(map[uint]time.Time, len(query.Memberships))
	for _, membership := range query.Memberships {
		joinedAt[membership.OrganizationID] = membership.JoinedAt
	}
	newStates := make([]MessageUserState, 0, len(records))
	for _, record := range records {
		if record.Kind == messagingdomain.MessageKindPrivate {
			continue
		}
		if _, exists := states[record.ID]; exists {
			continue
		}
		var readAt *time.Time
		if record.Kind == messagingdomain.MessageKindAnnouncement && record.PublishAt != nil && joinedAt[record.OrganizationID].After(*record.PublishAt) {
			initializedAt := query.Now
			readAt = &initializedAt
		}
		newStates = append(newStates, MessageUserState{MessageID: record.ID, UserID: query.UserID, ReadAt: readAt})
	}
	if len(newStates) == 0 {
		return nil
	}
	return repository.connection(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "message_id"}, {Name: "user_id"}}, DoNothing: true}).Create(&newStates).Error
}

func messageIDs(records []Message) []uint {
	ids := make([]uint, len(records))
	for index, record := range records {
		ids[index] = record.ID
	}
	return ids
}

func membershipOrganizationIDs(memberships []application.OrganizationMembership) []uint {
	ids := make([]uint, len(memberships))
	for index, membership := range memberships {
		ids[index] = membership.OrganizationID
	}
	return uniqueUintIDs(ids)
}

func inboxNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func (repository *Repository) ClaimOutbox(ctx context.Context, claim application.OutboxClaim) ([]messagingdomain.MessageOutbox, error) {
	if claim.WorkerID == "" || claim.Lease <= 0 || claim.Limit < 1 {
		return nil, application.ErrStateConflict
	}
	claim.Now = inboxNow(claim.Now)
	var claimed []MessageOutbox
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("((status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND lease_expires_at <= ?))", messagingdomain.OutboxStatusPending, claim.Now, messagingdomain.OutboxStatusPublishing, claim.Now).
			Where("NOT EXISTS (SELECT 1 FROM message_outboxes AS previous WHERE previous.message_copy_id = message_outboxes.message_copy_id AND previous.aggregate_version < message_outboxes.aggregate_version AND previous.status <> ?)", messagingdomain.OutboxStatusPublished).
			Order("message_copy_id asc, aggregate_version asc, id asc").
			Limit(claim.Limit)
		if err := query.Find(&claimed).Error; err != nil {
			return err
		}
		leaseExpiresAt := claim.Now.Add(claim.Lease)
		for index := range claimed {
			if err := tx.Model(&MessageOutbox{}).Where("id = ?", claimed[index].ID).Updates(map[string]any{"status": messagingdomain.OutboxStatusPublishing, "worker_id": claim.WorkerID, "lease_expires_at": leaseExpiresAt}).Error; err != nil {
				return err
			}
			claimed[index].Status = messagingdomain.OutboxStatusPublishing
			claimed[index].WorkerID = claim.WorkerID
			claimed[index].LeaseExpiresAt = &leaseExpiresAt
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outboxesToDomain(claimed), nil
}

func (repository *Repository) RenewOutboxLease(ctx context.Context, lease application.OutboxLease) (bool, error) {
	if lease.ID == 0 || lease.WorkerID == "" || lease.Lease <= 0 {
		return false, application.ErrLeaseNotHeld
	}
	lease.Now = inboxNow(lease.Now)
	expiresAt := lease.Now.Add(lease.Lease)
	result := repository.connection(ctx).Model(&MessageOutbox{}).
		Where("id = ? AND status = ? AND worker_id = ? AND lease_expires_at > ?", lease.ID, messagingdomain.OutboxStatusPublishing, lease.WorkerID, lease.Now).
		Update("lease_expires_at", expiresAt)
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) MarkOutboxPublished(ctx context.Context, lease application.OutboxLease) (bool, error) {
	if lease.ID == 0 || lease.WorkerID == "" {
		return false, application.ErrLeaseNotHeld
	}
	lease.Now = inboxNow(lease.Now)
	result := repository.connection(ctx).Model(&MessageOutbox{}).
		Where("id = ? AND status = ? AND worker_id = ? AND lease_expires_at > ?", lease.ID, messagingdomain.OutboxStatusPublishing, lease.WorkerID, lease.Now).
		Updates(map[string]any{"status": messagingdomain.OutboxStatusPublished, "published_at": lease.Now, "worker_id": "", "lease_expires_at": nil, "next_retry_at": nil})
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) RecordOutboxFailure(ctx context.Context, failure application.OutboxFailure) (bool, error) {
	if failure.ID == 0 || failure.WorkerID == "" || strings.TrimSpace(failure.FailureCode) == "" {
		return false, application.ErrLeaseNotHeld
	}
	failure.Now = inboxNow(failure.Now)
	status := messagingdomain.OutboxStatusPending
	nextRetryAt := failure.RetryAt
	if failure.Dead {
		status = messagingdomain.OutboxStatusDead
		nextRetryAt = nil
	}
	result := repository.connection(ctx).Model(&MessageOutbox{}).
		Where("id = ? AND status = ? AND worker_id = ? AND lease_expires_at > ?", failure.ID, messagingdomain.OutboxStatusPublishing, failure.WorkerID, failure.Now).
		Updates(map[string]any{"status": status, "retry_attempt": gorm.Expr("retry_attempt + 1"), "next_retry_at": nextRetryAt, "worker_id": "", "lease_expires_at": nil, "last_failure_code": failure.FailureCode})
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) ReplayOutbox(ctx context.Context, id uint) (bool, error) {
	if id == 0 {
		return false, application.ErrNotFound
	}
	result := repository.connection(ctx).Model(&MessageOutbox{}).Where("id = ? AND status = ?", id, messagingdomain.OutboxStatusDead).Updates(map[string]any{"status": messagingdomain.OutboxStatusPending, "retry_attempt": 0, "next_retry_at": nil, "worker_id": "", "lease_expires_at": nil, "last_failure_code": ""})
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) ListOutboxes(ctx context.Context, query application.OutboxListQuery) ([]messagingdomain.MessageOutbox, int64, error) {
	db := repository.connection(ctx).Model(&MessageOutbox{})
	if len(query.Statuses) > 0 {
		db = db.Where("status IN ?", query.Statuses)
	}
	if len(query.OrganizationIDs) > 0 {
		db = db.Where("organization_id IN ?", uniqueUintIDs(query.OrganizationIDs))
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []MessageOutbox
	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	}
	if query.Offset > 0 {
		db = db.Offset(query.Offset)
	}
	if err := db.Order("created_at desc, id desc").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return outboxesToDomain(records), total, nil
}

func outboxToDomain(record MessageOutbox) messagingdomain.MessageOutbox {
	event := messagingdomain.MessageEvent{EventID: record.EventID, EventName: record.EventName, EventVersion: record.EventVersion, MessageCopyID: record.MessageCopyID, OrganizationID: record.OrganizationID, OccurredAt: record.OccurredAt, AggregateVersion: record.AggregateVersion}
	return messagingdomain.MessageOutbox{ID: record.ID, Event: event, Status: record.Status, RetryAttempt: record.RetryAttempt, NextRetryAt: record.NextRetryAt, WorkerID: record.WorkerID, LeaseExpiresAt: record.LeaseExpiresAt, LastFailureCode: record.LastFailureCode}
}

func outboxesToDomain(records []MessageOutbox) []messagingdomain.MessageOutbox {
	outboxes := make([]messagingdomain.MessageOutbox, len(records))
	for index, record := range records {
		outboxes[index] = outboxToDomain(record)
	}
	return outboxes
}

func (repository *Repository) ClaimEventConsumption(ctx context.Context, claim application.EventConsumptionClaim) (application.EventConsumption, bool, error) {
	if claim.ConsumerName == "" || claim.WorkerID == "" || claim.Lease <= 0 {
		return application.EventConsumption{}, false, application.ErrStateConflict
	}
	if err := messagingdomain.ValidateMessageEvent(claim.Event); err != nil {
		return application.EventConsumption{}, false, err
	}
	claim.Now = inboxNow(claim.Now)
	var claimed application.EventConsumption
	acquired := false
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		var record MessageEventConsumption
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("consumer_name = ? AND event_id = ?", claim.ConsumerName, claim.Event.EventID).First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			expiresAt := claim.Now.Add(claim.Lease)
			record = MessageEventConsumption{ConsumerName: claim.ConsumerName, EventID: claim.Event.EventID, MessageCopyID: claim.Event.MessageCopyID, AggregateVersion: claim.Event.AggregateVersion, Status: messagingdomain.EventConsumptionStatusSnapshotting, WorkerID: claim.WorkerID, SnapshotFence: 1, LeaseExpiresAt: &expiresAt}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
			claimed, acquired = consumptionToApplication(record), true
			return nil
		}
		if err != nil {
			return err
		}
		if record.MessageCopyID != claim.Event.MessageCopyID || record.AggregateVersion != claim.Event.AggregateVersion {
			return application.ErrStateConflict
		}
		consumption := messagingdomain.EventConsumption{ConsumerName: record.ConsumerName, EventID: record.EventID, MessageCopyID: record.MessageCopyID, Status: record.Status, WorkerID: record.WorkerID, SnapshotFence: record.SnapshotFence, LeaseExpiresAt: record.LeaseExpiresAt, FailureCode: record.FailureCode, AudienceObservedCount: record.AudienceObservedCount}
		if err := messagingdomain.ClaimSnapshotLease(&consumption, claim.WorkerID, claim.Now, claim.Lease); err != nil {
			if errors.Is(err, messagingdomain.ErrSnapshotLeaseHeld) || errors.Is(err, messagingdomain.ErrSnapshotAlreadyCompleted) {
				claimed = consumptionToApplication(record)
				return nil
			}
			return err
		}
		record.Status = consumption.Status
		record.WorkerID = consumption.WorkerID
		record.SnapshotFence = consumption.SnapshotFence
		record.LeaseExpiresAt = consumption.LeaseExpiresAt
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		claimed, acquired = consumptionToApplication(record), true
		return nil
	})
	return claimed, acquired, err
}

func (repository *Repository) RenewEventConsumptionLease(ctx context.Context, lease application.EventConsumptionLease) (bool, error) {
	if lease.ConsumerName == "" || lease.EventID == "" || lease.WorkerID == "" || lease.SnapshotFence == 0 || lease.Lease <= 0 {
		return false, application.ErrLeaseNotHeld
	}
	lease.Now = inboxNow(lease.Now)
	result := repository.connection(ctx).Model(&MessageEventConsumption{}).
		Where("consumer_name = ? AND event_id = ? AND status = ? AND worker_id = ? AND snapshot_fence = ? AND lease_expires_at > ?", lease.ConsumerName, lease.EventID, messagingdomain.EventConsumptionStatusSnapshotting, lease.WorkerID, lease.SnapshotFence, lease.Now).
		Update("lease_expires_at", lease.Now.Add(lease.Lease))
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) ResetIncompleteAudienceSnapshot(ctx context.Context, lease application.EventConsumptionLease) (bool, error) {
	if lease.ConsumerName == "" || lease.EventID == "" || lease.WorkerID == "" || lease.SnapshotFence == 0 || lease.Lease <= 0 {
		return false, application.ErrLeaseNotHeld
	}
	lease.Now = inboxNow(lease.Now)
	reset := false
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		var consumption MessageEventConsumption
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("consumer_name = ? AND event_id = ? AND status = ? AND worker_id = ? AND snapshot_fence = ? AND lease_expires_at > ?", lease.ConsumerName, lease.EventID, messagingdomain.EventConsumptionStatusSnapshotting, lease.WorkerID, lease.SnapshotFence, lease.Now).First(&consumption).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if consumption.SnapshotComplete {
			return application.ErrStateConflict
		}
		if err := tx.Where("consumer_name = ? AND event_id = ?", lease.ConsumerName, lease.EventID).Delete(&MessageEventDelivery{}).Error; err != nil {
			return err
		}
		reset = true
		return nil
	})
	return reset, err
}

func (repository *Repository) MarkAudienceSnapshotComplete(ctx context.Context, lease application.EventConsumptionLease) (bool, error) {
	if lease.ConsumerName == "" || lease.EventID == "" || lease.WorkerID == "" || lease.SnapshotFence == 0 || lease.Lease <= 0 {
		return false, application.ErrLeaseNotHeld
	}
	lease.Now = inboxNow(lease.Now)
	result := repository.connection(ctx).Model(&MessageEventConsumption{}).
		Where("consumer_name = ? AND event_id = ? AND status = ? AND worker_id = ? AND snapshot_fence = ? AND lease_expires_at > ?", lease.ConsumerName, lease.EventID, messagingdomain.EventConsumptionStatusSnapshotting, lease.WorkerID, lease.SnapshotFence, lease.Now).
		Update("snapshot_complete", true)
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) PersistAudienceDeliveryBatch(ctx context.Context, batch application.AudienceDeliveryBatch) (bool, error) {
	if batch.ConsumerName == "" || batch.EventID == "" || batch.WorkerID == "" || batch.SnapshotFence == 0 || batch.MessageCopyID == 0 || batch.ExpiresAt.IsZero() {
		return false, application.ErrLeaseNotHeld
	}
	if len(batch.UserIDs) > 500 {
		return false, application.ErrAudienceBatchTooLarge
	}
	userIDs := uniqueUintIDs(batch.UserIDs)
	if len(userIDs) != len(batch.UserIDs) {
		return false, application.ErrStateConflict
	}
	batch.Now = inboxNow(batch.Now)
	var claimed int64
	if err := repository.connection(ctx).Model(&MessageEventConsumption{}).
		Where("consumer_name = ? AND event_id = ? AND message_copy_id = ? AND status = ? AND worker_id = ? AND snapshot_fence = ? AND lease_expires_at > ?", batch.ConsumerName, batch.EventID, batch.MessageCopyID, messagingdomain.EventConsumptionStatusSnapshotting, batch.WorkerID, batch.SnapshotFence, batch.Now).
		Count(&claimed).Error; err != nil {
		return false, err
	}
	if claimed == 0 {
		return false, nil
	}
	if len(userIDs) == 0 {
		return true, nil
	}
	deliveries := make([]MessageEventDelivery, len(userIDs))
	for index, userID := range userIDs {
		deliveries[index] = MessageEventDelivery{ConsumerName: batch.ConsumerName, EventID: batch.EventID, MessageCopyID: batch.MessageCopyID, UserID: userID, ExpiresAt: batch.ExpiresAt.UTC()}
	}
	err := repository.connection(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "consumer_name"}, {Name: "event_id"}, {Name: "user_id"}}, DoNothing: true}).Create(&deliveries).Error
	return err == nil, err
}

func (repository *Repository) DiscardAudienceDeliverySnapshot(ctx context.Context, lease application.EventConsumptionLease) (bool, error) {
	if lease.ConsumerName == "" || lease.EventID == "" || lease.WorkerID == "" || lease.SnapshotFence == 0 {
		return false, application.ErrLeaseNotHeld
	}
	discarded := false
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		var consumption MessageEventConsumption
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("consumer_name = ? AND event_id = ? AND status = ? AND worker_id = ? AND snapshot_fence = ?", lease.ConsumerName, lease.EventID, messagingdomain.EventConsumptionStatusSnapshotting, lease.WorkerID, lease.SnapshotFence).First(&consumption).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if consumption.SnapshotComplete {
			return application.ErrStateConflict
		}
		if err := tx.Where("consumer_name = ? AND event_id = ?", lease.ConsumerName, lease.EventID).Delete(&MessageEventDelivery{}).Error; err != nil {
			return err
		}
		discarded = true
		return nil
	})
	return discarded, err
}

func (repository *Repository) FailEventConsumption(ctx context.Context, failure application.EventConsumptionFailure) (bool, error) {
	if failure.ConsumerName == "" || failure.EventID == "" || failure.WorkerID == "" || failure.SnapshotFence == 0 || strings.TrimSpace(failure.FailureCode) == "" {
		return false, application.ErrLeaseNotHeld
	}
	failure.Now = inboxNow(failure.Now)
	result := repository.connection(ctx).Model(&MessageEventConsumption{}).
		Where("consumer_name = ? AND event_id = ? AND status = ? AND worker_id = ? AND snapshot_fence = ? AND lease_expires_at > ?", failure.ConsumerName, failure.EventID, messagingdomain.EventConsumptionStatusSnapshotting, failure.WorkerID, failure.SnapshotFence, failure.Now).
		Updates(map[string]any{"status": messagingdomain.EventConsumptionStatusFailed, "worker_id": "", "lease_expires_at": nil, "snapshot_complete": false, "failure_code": failure.FailureCode, "audience_observed_count": failure.AudienceObservedCount})
	return result.RowsAffected != 0, result.Error
}

func (repository *Repository) FinalizeEventConsumption(ctx context.Context, completion application.EventConsumptionCompletion) (application.EventConsumptionFinalization, error) {
	if completion.ConsumerName == "" || completion.EventID == "" || completion.WorkerID == "" || completion.SnapshotFence == 0 || completion.MessageCopyID == 0 || completion.AggregateVersion == 0 {
		return application.EventConsumptionFinalization{}, application.ErrLeaseNotHeld
	}
	completion.Now = inboxNow(completion.Now)
	finalization := application.EventConsumptionFinalization{}
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		var consumption MessageEventConsumption
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("consumer_name = ? AND event_id = ?", completion.ConsumerName, completion.EventID).First(&consumption).Error; err != nil {
			return mapRepositoryError(err)
		}
		if consumption.MessageCopyID != completion.MessageCopyID || consumption.AggregateVersion != completion.AggregateVersion || consumption.Status != messagingdomain.EventConsumptionStatusSnapshotting || consumption.WorkerID != completion.WorkerID || consumption.SnapshotFence != completion.SnapshotFence || consumption.LeaseExpiresAt == nil || !consumption.LeaseExpiresAt.After(completion.Now) {
			return application.ErrLeaseNotHeld
		}
		cursor := MessageEventConsumerCursor{ConsumerName: completion.ConsumerName, MessageCopyID: completion.MessageCopyID}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "consumer_name"}, {Name: "message_copy_id"}}, DoNothing: true}).Create(&cursor).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("consumer_name = ? AND message_copy_id = ?", completion.ConsumerName, completion.MessageCopyID).First(&cursor).Error; err != nil {
			return err
		}
		if cursor.AggregateVersion > completion.AggregateVersion {
			if err := tx.Model(&MessageEventConsumption{}).Where("id = ?", consumption.ID).Updates(map[string]any{"status": messagingdomain.EventConsumptionStatusSuperseded, "worker_id": "", "lease_expires_at": nil, "completed_at": completion.Now}).Error; err != nil {
				return err
			}
			finalization = application.EventConsumptionFinalization{Status: messagingdomain.EventConsumptionStatusSuperseded, Completed: true}
			return nil
		}
		if cursor.AggregateVersion < completion.AggregateVersion {
			if err := tx.Model(&MessageEventConsumerCursor{}).Where("id = ?", cursor.ID).Update("aggregate_version", completion.AggregateVersion).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&MessageEventConsumption{}).Where("id = ?", consumption.ID).Updates(map[string]any{"status": messagingdomain.EventConsumptionStatusCompleted, "worker_id": "", "lease_expires_at": nil, "completed_at": completion.Now}).Error; err != nil {
			return err
		}
		finalization = application.EventConsumptionFinalization{Status: messagingdomain.EventConsumptionStatusCompleted, Completed: true}
		return nil
	})
	return finalization, err
}

func (repository *Repository) ListAudienceDeliveryUserIDs(ctx context.Context, consumerName, eventID string) ([]uint, error) {
	var userIDs []uint
	err := repository.connection(ctx).Model(&MessageEventDelivery{}).Where("consumer_name = ? AND event_id = ?", consumerName, eventID).Order("user_id asc").Pluck("user_id", &userIDs).Error
	return userIDs, err
}

func (repository *Repository) CleanupAudienceDeliveries(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if limit < 1 {
		return 0, nil
	}
	var ids []uint
	if err := repository.connection(ctx).Model(&MessageEventDelivery{}).Where("expires_at <= ?", cutoff.UTC()).Order("id asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := repository.connection(ctx).Where("id IN ?", ids).Delete(&MessageEventDelivery{})
	return result.RowsAffected, result.Error
}

func consumptionToApplication(record MessageEventConsumption) application.EventConsumption {
	return application.EventConsumption{ConsumerName: record.ConsumerName, EventID: record.EventID, MessageCopyID: record.MessageCopyID, AggregateVersion: record.AggregateVersion, Status: record.Status, WorkerID: record.WorkerID, SnapshotFence: record.SnapshotFence, SnapshotComplete: record.SnapshotComplete, LeaseExpiresAt: record.LeaseExpiresAt, FailureCode: record.FailureCode, AudienceObservedCount: record.AudienceObservedCount, CompletedAt: record.CompletedAt}
}

func (repository *Repository) RecordConsumerDeadLetter(ctx context.Context, input application.ConsumerDeadLetterInput) (application.ConsumerDeadLetter, error) {
	if input.ConsumerName == "" || input.OriginalQueue == "" || strings.TrimSpace(input.FailureCode) == "" {
		return application.ConsumerDeadLetter{}, application.ErrStateConflict
	}
	if input.Invalid {
		if !validDeadLetterFingerprint(input.Fingerprint) {
			return application.ConsumerDeadLetter{}, application.ErrConsumerDeadLetterInvalid
		}
		input.Event = invalidEventProjection(input.Fingerprint, input.Now)
		input.Event.EventID = "invalid:" + input.Fingerprint
	} else if err := messagingdomain.ValidateMessageEvent(input.Event); err != nil {
		return application.ConsumerDeadLetter{}, err
	}
	input.Now = inboxNow(input.Now)
	var recorded MessageConsumerDeadLetter
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("consumer_name = ? AND event_id = ?", input.ConsumerName, input.Event.EventID).First(&recorded).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			recorded = MessageConsumerDeadLetter{ConsumerName: input.ConsumerName, EventID: input.Event.EventID, OriginalQueue: input.OriginalQueue, EventName: input.Event.EventName, EventVersion: input.Event.EventVersion, OccurredAt: input.Event.OccurredAt, MessageCopyID: input.Event.MessageCopyID, OrganizationID: input.Event.OrganizationID, AggregateVersion: input.Event.AggregateVersion, RetryAttempt: input.RetryAttempt, Status: messagingdomain.ConsumerDLQStatusPending, LastFailureCode: input.FailureCode, AudienceObservedCount: input.AudienceObservedCount, Invalid: input.Invalid, Fingerprint: input.Fingerprint, Replayable: !input.Invalid}
			if err := tx.Create(&recorded).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if recorded.Invalid != input.Invalid || recorded.Fingerprint != input.Fingerprint || recorded.MessageCopyID != input.Event.MessageCopyID || recorded.AggregateVersion != input.Event.AggregateVersion || recorded.EventName != input.Event.EventName || recorded.EventVersion != input.Event.EventVersion || recorded.OrganizationID != input.Event.OrganizationID || !recorded.OccurredAt.Equal(input.Event.OccurredAt) {
				return application.ErrStateConflict
			}
			if recorded.Status == messagingdomain.ConsumerDLQStatusReplayed {
				deadLetter := deadLetterToDomain(recorded)
				if err := messagingdomain.ReopenConsumerDLQAfterFailure(&deadLetter, input.FailureCode); err != nil {
					return err
				}
				recorded.Status = deadLetter.Status
				recorded.ReplayCycle = deadLetter.ReplayCycle
				recorded.ReplayLeaseOwner = ""
				recorded.ReplayLeaseExpiresAt = nil
				recorded.FinalizedAt = nil
			}
			recorded.OriginalQueue = input.OriginalQueue
			recorded.RetryAttempt = input.RetryAttempt
			recorded.LastFailureCode = input.FailureCode
			recorded.AudienceObservedCount = input.AudienceObservedCount
			if err := tx.Save(&recorded).Error; err != nil {
				return err
			}
		}
		return tx.Model(&MessageEventDelivery{}).Where("consumer_name = ? AND event_id = ?", input.ConsumerName, input.Event.EventID).Updates(map[string]any{"consumer_dead_letter_id": recorded.ID, "expires_at": pendingDeadLetterDeliveryExpiry}).Error
	})
	if err != nil {
		return application.ConsumerDeadLetter{}, err
	}
	return deadLetterToApplication(recorded), nil
}

func (repository *Repository) ListConsumerDeadLetters(ctx context.Context, query application.ConsumerDeadLetterListQuery) ([]application.ConsumerDeadLetter, int64, error) {
	db := repository.connection(ctx).Model(&MessageConsumerDeadLetter{})
	if len(query.Statuses) > 0 {
		db = db.Where("status IN ?", query.Statuses)
	}
	if query.ConsumerName != "" {
		db = db.Where("consumer_name = ?", query.ConsumerName)
	}
	if len(query.OrganizationIDs) > 0 {
		db = db.Where("organization_id IN ?", uniqueUintIDs(query.OrganizationIDs))
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []MessageConsumerDeadLetter
	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	}
	if query.Offset > 0 {
		db = db.Offset(query.Offset)
	}
	if err := db.Order("created_at desc, id desc").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	deadLetters := make([]application.ConsumerDeadLetter, len(records))
	for index, record := range records {
		deadLetters[index] = deadLetterToApplication(record)
	}
	return deadLetters, total, nil
}

func (repository *Repository) ClaimConsumerDeadLetterReplay(ctx context.Context, claim application.ConsumerDeadLetterReplayClaim) (application.ConsumerDeadLetter, bool, error) {
	if claim.ID == 0 || claim.WorkerID == "" || claim.Lease <= 0 {
		return application.ConsumerDeadLetter{}, false, application.ErrStateConflict
	}
	claim.Now = inboxNow(claim.Now)
	var claimed MessageConsumerDeadLetter
	acquired := false
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&claimed, claim.ID).Error; err != nil {
			return mapRepositoryError(err)
		}
		if claimed.Invalid || !claimed.Replayable {
			return application.ErrConsumerDeadLetterInvalid
		}
		deadLetter := deadLetterToDomain(claimed)
		if err := messagingdomain.BeginConsumerDLQReplay(&deadLetter, claim.WorkerID, claim.Now, claim.Lease); err != nil {
			if errors.Is(err, messagingdomain.ErrConsumerDLQInvalid) {
				return nil
			}
			return err
		}
		claimed.Status = deadLetter.Status
		claimed.ReplayCycle = deadLetter.ReplayCycle
		claimed.ReplayLeaseFence = deadLetter.ReplayLeaseFence
		claimed.ReplayLeaseOwner = deadLetter.ReplayLeaseOwner
		claimed.ReplayLeaseExpiresAt = deadLetter.ReplayLeaseExpiresAt
		if err := tx.Save(&claimed).Error; err != nil {
			return err
		}
		acquired = true
		return nil
	})
	if err != nil {
		return application.ConsumerDeadLetter{}, false, err
	}
	return deadLetterToApplication(claimed), acquired, nil
}

func (repository *Repository) MarkConsumerDeadLetterReplayed(ctx context.Context, result application.ConsumerDeadLetterReplayResult) (bool, error) {
	if result.ID == 0 || result.WorkerID == "" || result.Fence == 0 {
		return false, application.ErrLeaseNotHeld
	}
	result.Now = inboxNow(result.Now)
	var deadLetter MessageConsumerDeadLetter
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deadLetter, result.ID).Error; err != nil {
			return mapRepositoryError(err)
		}
		view := deadLetterToDomain(deadLetter)
		if deadLetter.ReplayLeaseExpiresAt == nil || !deadLetter.ReplayLeaseExpiresAt.After(result.Now) {
			return application.ErrLeaseNotHeld
		}
		if err := messagingdomain.MarkConsumerDLQReplayed(&view, result.WorkerID, result.Fence); err != nil {
			return application.ErrLeaseNotHeld
		}
		deadLetter.Status = view.Status
		deadLetter.ReplayLeaseOwner = ""
		deadLetter.ReplayLeaseExpiresAt = nil
		deadLetter.FinalizedAt = &result.Now
		if err := tx.Save(&deadLetter).Error; err != nil {
			return err
		}
		return tx.Model(&MessageEventDelivery{}).Where("consumer_dead_letter_id = ?", deadLetter.ID).Update("expires_at", result.Now.Add(30*24*time.Hour)).Error
	})
	return err == nil, err
}

func (repository *Repository) ReturnConsumerDeadLetterPending(ctx context.Context, result application.ConsumerDeadLetterReplayResult) (bool, error) {
	if result.ID == 0 || result.WorkerID == "" || result.Fence == 0 {
		return false, application.ErrLeaseNotHeld
	}
	result.Now = inboxNow(result.Now)
	res := repository.connection(ctx).Model(&MessageConsumerDeadLetter{}).
		Where("id = ? AND status = ? AND replay_lease_owner = ? AND replay_lease_fence = ? AND replay_lease_expires_at > ?", result.ID, messagingdomain.ConsumerDLQStatusReplaying, result.WorkerID, result.Fence, result.Now).
		Updates(map[string]any{"status": messagingdomain.ConsumerDLQStatusPending, "replay_lease_owner": "", "replay_lease_expires_at": nil})
	return res.RowsAffected != 0, res.Error
}

func (repository *Repository) DiscardConsumerDeadLetter(ctx context.Context, id uint, now time.Time) (bool, error) {
	if id == 0 {
		return false, application.ErrNotFound
	}
	now = inboxNow(now)
	var deadLetter MessageConsumerDeadLetter
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deadLetter, id).Error; err != nil {
			return mapRepositoryError(err)
		}
		if deadLetter.Status != messagingdomain.ConsumerDLQStatusPending {
			return application.ErrStateConflict
		}
		deadLetter.Status = messagingdomain.ConsumerDLQStatusDiscarded
		deadLetter.FinalizedAt = &now
		if err := tx.Save(&deadLetter).Error; err != nil {
			return err
		}
		return tx.Model(&MessageEventDelivery{}).Where("consumer_dead_letter_id = ?", id).Update("expires_at", now.Add(30*24*time.Hour)).Error
	})
	return err == nil, err
}

func (repository *Repository) CleanupFinalConsumerDeadLetters(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if limit < 1 {
		return 0, nil
	}
	var ids []uint
	if err := repository.connection(ctx).Model(&MessageConsumerDeadLetter{}).Where("status IN ? AND finalized_at <= ?", []messagingdomain.ConsumerDLQStatus{messagingdomain.ConsumerDLQStatusReplayed, messagingdomain.ConsumerDLQStatusDiscarded}, cutoff.UTC()).Order("id asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	err := repository.connection(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("consumer_dead_letter_id IN ?", ids).Delete(&MessageEventDelivery{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", ids).Delete(&MessageConsumerDeadLetter{}).Error
	})
	if err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

var pendingDeadLetterDeliveryExpiry = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

func deadLetterToApplication(record MessageConsumerDeadLetter) application.ConsumerDeadLetter {
	event := messagingdomain.MessageEvent{EventID: record.EventID, EventName: record.EventName, EventVersion: record.EventVersion, MessageCopyID: record.MessageCopyID, OrganizationID: record.OrganizationID, OccurredAt: record.OccurredAt, AggregateVersion: record.AggregateVersion}
	return application.ConsumerDeadLetter{ID: record.ID, ConsumerName: record.ConsumerName, Event: event, OriginalQueue: record.OriginalQueue, RetryAttempt: record.RetryAttempt, Status: record.Status, ReplayCycle: record.ReplayCycle, ReplayLeaseFence: record.ReplayLeaseFence, ReplayLeaseOwner: record.ReplayLeaseOwner, ReplayLeaseExpiresAt: record.ReplayLeaseExpiresAt, LastFailureCode: record.LastFailureCode, AudienceObservedCount: record.AudienceObservedCount, Invalid: record.Invalid, Fingerprint: record.Fingerprint, Replayable: record.Replayable, FinalizedAt: record.FinalizedAt}
}

func deadLetterToDomain(record MessageConsumerDeadLetter) messagingdomain.ConsumerDeadLetter {
	return messagingdomain.ConsumerDeadLetter{ConsumerName: record.ConsumerName, EventID: record.EventID, Status: record.Status, ReplayCycle: record.ReplayCycle, ReplayLeaseFence: record.ReplayLeaseFence, ReplayLeaseOwner: record.ReplayLeaseOwner, ReplayLeaseExpiresAt: record.ReplayLeaseExpiresAt, LastFailureCode: record.LastFailureCode, AudienceObservedCount: record.AudienceObservedCount}
}

func validDeadLetterFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func invalidEventProjection(fingerprint string, now time.Time) messagingdomain.MessageEvent {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return messagingdomain.MessageEvent{EventID: "invalid:" + fingerprint, EventName: messagingdomain.EventNameInvalid, EventVersion: 1, OccurredAt: now.UTC()}
}
