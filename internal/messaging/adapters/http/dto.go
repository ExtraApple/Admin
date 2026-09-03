package httpadapter

import (
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

// MessageDTO is the public message representation. Markdown, object-storage
// paths, broker payloads, and credentials are intentionally not represented.
type MessageDTO struct {
	ID               uint       `json:"id"`
	LogicalID        string     `json:"logical_id"`
	OrganizationID   uint       `json:"organization_id"`
	SenderID         uint       `json:"sender_id"`
	CategoryID       uint       `json:"category_id"`
	Kind             string     `json:"kind"`
	Status           string     `json:"status"`
	Title            string     `json:"title"`
	BodyHTML         string     `json:"body_html"`
	PublishAt        *time.Time `json:"publish_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	ReadAt           *time.Time `json:"read_at,omitempty"`
	Read             bool       `json:"read"`
	AggregateVersion uint64     `json:"aggregate_version"`
}

func toMessageDTO(item application.InboxMessage) MessageDTO {
	message := item.Message
	bodyHTML := message.BodyHTML
	if message.Status == domain.MessageStatusRevoked || message.Status == domain.MessageStatusExpired {
		bodyHTML = ""
	}
	return MessageDTO{
		ID: message.ID, LogicalID: message.LogicalID, OrganizationID: message.OrganizationID, SenderID: message.SenderID,
		CategoryID: message.CategoryID, Kind: string(message.Kind), Status: string(message.Status), Title: message.Title,
		BodyHTML: bodyHTML, PublishAt: message.PublishAt, ExpiresAt: message.ExpiresAt, RevokedAt: message.RevokedAt,
		ReadAt: item.ReadAt, Read: item.ReadAt != nil, AggregateVersion: message.AggregateVersion,
	}
}

func toMessageDTOs(items []application.InboxMessage) []MessageDTO {
	result := make([]MessageDTO, len(items))
	for index, item := range items {
		result[index] = toMessageDTO(item)
	}
	return result
}

type SendPrivateMessageRequest struct {
	RecipientID uint   `json:"recipient_id" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Markdown    string `json:"markdown" binding:"required"`
	ImageIDs    []uint `json:"image_ids,omitempty"`
}

type InboxResponse struct {
	Items  []MessageDTO `json:"items"`
	Total  int64        `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
}

type UnreadCountResponse struct {
	Private      int64 `json:"private"`
	Broadcast    int64 `json:"broadcast"`
	Announcement int64 `json:"announcement"`
	Total        int64 `json:"total"`
}

type DynamicAudienceTarget struct {
	OrganizationID uint   `json:"organization_id" binding:"required"`
	Type           string `json:"type" binding:"required"`
	RoleID         uint   `json:"role_id,omitempty"`
}

func (target DynamicAudienceTarget) domain() application.DynamicAudienceTarget {
	return application.DynamicAudienceTarget{OrganizationID: target.OrganizationID, Type: domain.AudienceType(target.Type), RoleID: target.RoleID}
}

func dynamicAudienceTargets(targets []DynamicAudienceTarget) []application.DynamicAudienceTarget {
	result := make([]application.DynamicAudienceTarget, len(targets))
	for index, target := range targets {
		result[index] = target.domain()
	}
	return result
}

type CreateBroadcastRequest struct {
	CategoryCode string                  `json:"category_code" binding:"required"`
	Title        string                  `json:"title" binding:"required"`
	Markdown     string                  `json:"markdown" binding:"required"`
	Targets      []DynamicAudienceTarget `json:"targets"`
	AllUsers     bool                    `json:"all_users,omitempty"`
	ImageIDs     []uint                  `json:"image_ids,omitempty"`
}
type CreateAnnouncementRequest struct {
	CategoryCode string                  `json:"category_code" binding:"required"`
	Title        string                  `json:"title" binding:"required"`
	Markdown     string                  `json:"markdown" binding:"required"`
	Targets      []DynamicAudienceTarget `json:"targets"`
	AllUsers     bool                    `json:"all_users,omitempty"`
	PublishAt    *time.Time              `json:"publish_at,omitempty"`
	ExpiresAt    *time.Time              `json:"expires_at,omitempty"`
	ImageIDs     []uint                  `json:"image_ids,omitempty"`
}

type EditAnnouncementRequest struct {
	CategoryCode string                  `json:"category_code,omitempty"`
	Title        string                  `json:"title" binding:"required"`
	Markdown     string                  `json:"markdown" binding:"required"`
	Targets      []DynamicAudienceTarget `json:"targets"`
	AllUsers     bool                    `json:"all_users,omitempty"`
	ExpiresAt    *time.Time              `json:"expires_at,omitempty"`
	ImageIDs     []uint                  `json:"image_ids,omitempty"`
}
type PublishAnnouncementRequest struct {
	ImageIDs []uint `json:"image_ids,omitempty"`
}
type MessageCategoryDTO struct {
	ID             uint   `json:"id"`
	OrganizationID uint   `json:"organization_id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Sort           int    `json:"sort"`
	Enabled        bool   `json:"enabled"`
}

type CreateMessageCategoryRequest struct {
	OrganizationID uint   `json:"organization_id" binding:"required"`
	Code           string `json:"code" binding:"required"`
	Name           string `json:"name" binding:"required"`
	Sort           int    `json:"sort"`
	Enabled        bool   `json:"enabled"`
}

type UpdateMessageCategoryRequest struct {
	Name    *string `json:"name,omitempty"`
	Sort    *int    `json:"sort,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

type ManagedMessageListResponse struct {
	Items  []MessageDTO `json:"items"`
	Total  int64        `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
}

type CategoryListResponse struct {
	Items []MessageCategoryDTO `json:"items"`
	Total int64                `json:"total"`
}

type MessageOutboxDTO struct {
	ID              uint                `json:"id"`
	Event           domain.MessageEvent `json:"event"`
	Status          string              `json:"status"`
	RetryAttempt    int                 `json:"retry_attempt"`
	NextRetryAt     *time.Time          `json:"next_retry_at,omitempty"`
	LastFailureCode string              `json:"last_failure_code,omitempty"`
}

type MessageOutboxListResponse struct {
	Items  []MessageOutboxDTO `json:"items"`
	Total  int64              `json:"total"`
	Offset int                `json:"offset"`
	Limit  int                `json:"limit"`
}

type ConsumerDeadLetterListResponse struct {
	Items  []ConsumerDeadLetterDTO `json:"items"`
	Total  int64                   `json:"total"`
	Offset int                     `json:"offset"`
	Limit  int                     `json:"limit"`
}

type MessageImageUploadResponse struct {
	ID        uint      `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ConsumerDeadLetterDTO struct {
	ID                    uint       `json:"id"`
	ConsumerName          string     `json:"consumer_name"`
	EventID               string     `json:"event_id"`
	OriginalQueue         string     `json:"original_queue"`
	EventName             string     `json:"event_name"`
	EventVersion          uint       `json:"event_version"`
	MessageCopyID         uint       `json:"message_copy_id"`
	OrganizationID        uint       `json:"organization_id"`
	AggregateVersion      uint64     `json:"aggregate_version"`
	RetryAttempt          int        `json:"retry_attempt"`
	Status                string     `json:"status"`
	ReplayCycle           uint       `json:"replay_cycle"`
	LastFailureCode       string     `json:"last_failure_code"`
	AudienceObservedCount int        `json:"audience_observed_count"`
	Invalid               bool       `json:"invalid"`
	Fingerprint           string     `json:"fingerprint,omitempty"`
	Replayable            bool       `json:"replayable"`
	FinalizedAt           *time.Time `json:"finalized_at,omitempty"`
}

type WebSocketTicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"`
}
