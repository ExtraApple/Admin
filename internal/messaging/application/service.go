package application

import (
	"context"
	"errors"
	"io"
	"sort"
	"time"

	"github.com/google/uuid"

	"admin/internal/messaging/domain"
)

var (
	ErrRecipientUnavailable          = errors.New("private message recipient is unavailable")
	ErrMessagingDependency           = errors.New("messaging dependency is unavailable")
	ErrInboxRequestInvalid           = errors.New("inbox request is invalid")
	ErrRevokeNotAllowed              = errors.New("message revoke is not allowed")
	ErrPermissionDenied              = errors.New("messaging permission is denied")
	ErrOrganizationNotManaged        = errors.New("message organization is outside the management scope")
	ErrCategoryInUse                 = errors.New("message category is referenced")
	ErrCategoryUnavailable           = errors.New("message category is unavailable")
	ErrBroadcastAudienceInvalid      = errors.New("broadcast audience is invalid")
	ErrMessageImmutable              = errors.New("message kind is immutable")
	ErrNotificationProjectionInvalid = errors.New("notification projection request is invalid")
)

type Clock interface {
	Now() time.Time
}

type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type Dependencies struct {
	// wiring: required —— 缺失时消息查询、私信发送、编辑与撤销返回 messaging_dependency_unavailable
	Messages MessageStore
	// wiring: required —— 缺失时分类管理与公告、群发创建返回 messaging_dependency_unavailable
	Categories CategoryStore
	// wiring: required —— 缺失时收件箱查询、已读与删除返回 messaging_dependency_unavailable
	Inbox InboxStore
	// wiring: required —— 缺失时通知投影查询返回 messaging_dependency_unavailable
	Notifications NotificationStore
	// wiring: required —— 缺失时 Outbox 管理与死信重放返回 messaging_dependency_unavailable
	Outboxes OutboxStore
	// wiring: required —— 缺失时 Consumer 死信查询与丢弃返回 messaging_dependency_unavailable
	ConsumerDeadLetters ConsumerDeadLetterStore
	// wiring: required —— 缺失时死信重放返回 messaging_dependency_unavailable；Broker 未配置时组合根赋 nil，仅该路径不可用
	ConsumerDeadLetterReplay *ConsumerDeadLetterReplayService
	// wiring: required —— 缺失时私信发送与通知投影返回 messaging_dependency_unavailable
	Identity IdentityReader
	// wiring: required —— 缺失时受众解析失败，收件箱、公告、群发与撤销返回 messaging_dependency_unavailable
	Organizations OrganizationAudienceReader
	// wiring: required —— 缺失时权限与数据范围校验失败，分类、公告、群发与撤销返回 messaging_dependency_unavailable
	Authorization AuthorizationReader
	// wiring: required —— 缺失时消息图片上传、绑定与读取返回 messaging_dependency_unavailable
	Files MessageImageFiles
	// wiring: optional —— 缺失时 constructor 兜底 directTransactionRunner{}，事务退化为直通执行
	Transactions TransactionRunner
	// wiring: optional —— 缺失时 constructor 兜底 domain.DefaultContentLimits()（标题 100 / 正文 20000）
	ContentLimits domain.ContentLimits
	// wiring: optional —— 缺失时 constructor 兜底 ClockFunc(time.Now)
	Clock Clock
}

type Service struct {
	messages                 MessageStore
	categories               CategoryStore
	inbox                    InboxStore
	notifications            NotificationStore
	outboxes                 OutboxStore
	consumerDeadLetters      ConsumerDeadLetterStore
	consumerDeadLetterReplay *ConsumerDeadLetterReplayService
	identity                 IdentityReader
	organizations            OrganizationAudienceReader
	authorization            AuthorizationReader
	files                    MessageImageFiles
	transactions             TransactionRunner
	contentLimits            domain.ContentLimits
	clock                    Clock
}

func NewService(dependencies Dependencies) *Service {
	if dependencies.Clock == nil {
		dependencies.Clock = ClockFunc(time.Now)
	}
	if dependencies.Transactions == nil {
		dependencies.Transactions = directTransactionRunner{}
	}
	if dependencies.ContentLimits == (domain.ContentLimits{}) {
		dependencies.ContentLimits = domain.DefaultContentLimits()
	}
	return &Service{
		messages: dependencies.Messages, categories: dependencies.Categories, inbox: dependencies.Inbox,
		notifications: dependencies.Notifications, outboxes: dependencies.Outboxes,
		consumerDeadLetters: dependencies.ConsumerDeadLetters, consumerDeadLetterReplay: dependencies.ConsumerDeadLetterReplay,
		identity: dependencies.Identity, organizations: dependencies.Organizations, authorization: dependencies.Authorization,
		files: dependencies.Files, transactions: dependencies.Transactions,
		contentLimits: dependencies.ContentLimits, clock: dependencies.Clock,
	}
}

func (service *Service) currentRoleIDs(ctx context.Context, userID uint, requested []uint) ([]uint, error) {
	if reader, ok := service.organizations.(UserRoleIDsReader); ok {
		return reader.RoleIDs(ctx, userID)
	}
	return append([]uint(nil), requested...), nil
}

type InboxQueryRequest struct {
	UserID     uint
	RoleIDs    []uint
	Kinds      []domain.MessageKind
	CategoryID uint
	Read       *bool
	Keyword    string
	Offset     int
	Limit      int
}

type InboxResult struct {
	Items []InboxMessage
	Total int64
}

func (service *Service) ListInbox(ctx context.Context, request InboxQueryRequest) (InboxResult, error) {
	if service == nil || service.inbox == nil || service.organizations == nil {
		return InboxResult{}, ErrMessagingDependency
	}
	if request.UserID == 0 || request.Offset < 0 || request.Limit < 0 {
		return InboxResult{}, ErrInboxRequestInvalid
	}
	memberships, err := service.organizations.Memberships(ctx, request.UserID)
	if err != nil {
		return InboxResult{}, err
	}
	roleIDs, err := service.currentRoleIDs(ctx, request.UserID, request.RoleIDs)
	if err != nil {
		return InboxResult{}, err
	}
	items, total, err := service.inbox.ListInbox(ctx, InboxQuery{UserID: request.UserID, Memberships: memberships, RoleIDs: roleIDs, Kinds: append([]domain.MessageKind(nil), request.Kinds...), CategoryID: request.CategoryID, Read: request.Read, Keyword: request.Keyword, Now: service.clock.Now().UTC(), Offset: request.Offset, Limit: request.Limit})
	if err != nil {
		return InboxResult{}, err
	}
	return InboxResult{Items: items, Total: total}, nil
}

func (service *Service) GetInboxMessage(ctx context.Context, request InboxIdentityRequest) (InboxMessage, error) {
	if service == nil || service.inbox == nil || service.organizations == nil {
		return InboxMessage{}, ErrMessagingDependency
	}
	if request.UserID == 0 || request.MessageID == 0 {
		return InboxMessage{}, ErrInboxRequestInvalid
	}
	memberships, err := service.organizations.Memberships(ctx, request.UserID)
	if err != nil {
		return InboxMessage{}, err
	}
	roleIDs, err := service.currentRoleIDs(ctx, request.UserID, request.RoleIDs)
	if err != nil {
		return InboxMessage{}, err
	}
	now := service.clock.Now().UTC()
	item, visible, err := service.inbox.FindInboxMessage(ctx, InboxIdentity{UserID: request.UserID, MessageID: request.MessageID, Memberships: memberships, RoleIDs: roleIDs, Now: now})
	if err != nil {
		return InboxMessage{}, err
	}
	if !visible {
		return InboxMessage{}, ErrNotFound
	}
	return item, nil
}

type InboxIdentityRequest struct {
	UserID    uint
	MessageID uint
	RoleIDs   []uint
}

type UnreadInboxRequest struct {
	UserID  uint
	RoleIDs []uint
}

func (service *Service) MarkInboxRead(ctx context.Context, request InboxIdentityRequest) error {
	if service == nil || service.inbox == nil || service.organizations == nil {
		return ErrMessagingDependency
	}
	if request.UserID == 0 || request.MessageID == 0 {
		return ErrInboxRequestInvalid
	}
	memberships, err := service.organizations.Memberships(ctx, request.UserID)
	if err != nil {
		return err
	}
	roleIDs, err := service.currentRoleIDs(ctx, request.UserID, request.RoleIDs)
	if err != nil {
		return err
	}
	changed, err := service.inbox.MarkInboxRead(ctx, InboxIdentity{UserID: request.UserID, MessageID: request.MessageID, Memberships: memberships, RoleIDs: roleIDs, Now: service.clock.Now().UTC()}, service.clock.Now().UTC())
	if err != nil {
		return err
	}
	if !changed {
		return ErrNotFound
	}
	return nil
}

func (service *Service) CountUnreadInbox(ctx context.Context, request UnreadInboxRequest) (UnreadInboxCount, error) {
	if service == nil || service.inbox == nil || service.organizations == nil {
		return UnreadInboxCount{}, ErrMessagingDependency
	}
	if request.UserID == 0 {
		return UnreadInboxCount{}, ErrInboxRequestInvalid
	}
	memberships, err := service.organizations.Memberships(ctx, request.UserID)
	if err != nil {
		return UnreadInboxCount{}, err
	}
	roleIDs, err := service.currentRoleIDs(ctx, request.UserID, request.RoleIDs)
	if err != nil {
		return UnreadInboxCount{}, err
	}
	return service.inbox.CountUnreadInbox(ctx, InboxQuery{UserID: request.UserID, Memberships: memberships, RoleIDs: roleIDs, Now: service.clock.Now().UTC()})
}

type MessageImageUploadRequest struct {
	UserID                uint
	FileName, ContentType string
	Size                  int64
	Reader                io.Reader
}

func (service *Service) UploadMessageImage(ctx context.Context, request MessageImageUploadRequest) (TemporaryMessageImage, error) {
	if service == nil || service.files == nil || request.UserID == 0 || request.Reader == nil || request.Size < 0 || request.FileName == "" || request.ContentType == "" {
		return TemporaryMessageImage{}, ErrInboxRequestInvalid
	}
	return service.files.UploadMessageImage(ctx, MessageImageUpload{UploaderID: request.UserID, FileName: request.FileName, ContentType: request.ContentType, Size: request.Size, Reader: request.Reader})
}

type MessageImageReadRequest struct {
	UserID    uint
	MessageID uint
	ImageID   uint
	RoleIDs   []uint
}

func (service *Service) OpenVisibleMessageImage(ctx context.Context, request MessageImageReadRequest) (MessageImageContent, error) {
	if service == nil || service.inbox == nil || service.organizations == nil || service.files == nil {
		return MessageImageContent{}, ErrMessagingDependency
	}
	if request.UserID == 0 || request.MessageID == 0 || request.ImageID == 0 {
		return MessageImageContent{}, ErrInboxRequestInvalid
	}
	memberships, err := service.organizations.Memberships(ctx, request.UserID)
	if err != nil {
		return MessageImageContent{}, err
	}
	roleIDs, err := service.currentRoleIDs(ctx, request.UserID, request.RoleIDs)
	if err != nil {
		return MessageImageContent{}, err
	}
	now := service.clock.Now().UTC()
	inbox, visible, err := service.inbox.FindInboxMessage(ctx, InboxIdentity{UserID: request.UserID, MessageID: request.MessageID, Memberships: memberships, RoleIDs: roleIDs, Now: now})
	if err != nil {
		return MessageImageContent{}, err
	}
	if !visible || inbox.Message.Status != domain.MessageStatusPublished {
		return MessageImageContent{}, ErrNotFound
	}
	return service.files.OpenMessageImage(ctx, VisibleMessageImage{ID: request.ImageID, MessageLogicalID: inbox.Message.LogicalID})
}
func (service *Service) DeletePrivateInbox(ctx context.Context, request InboxIdentityRequest) error {
	if service == nil || service.inbox == nil || service.organizations == nil {
		return ErrMessagingDependency
	}
	if request.UserID == 0 || request.MessageID == 0 {
		return ErrInboxRequestInvalid
	}
	memberships, err := service.organizations.Memberships(ctx, request.UserID)
	if err != nil {
		return err
	}
	roleIDs, err := service.currentRoleIDs(ctx, request.UserID, request.RoleIDs)
	if err != nil {
		return err
	}
	deleted, err := service.inbox.DeletePrivateInbox(ctx, InboxIdentity{UserID: request.UserID, MessageID: request.MessageID, Memberships: memberships, RoleIDs: roleIDs, Now: service.clock.Now().UTC()}, service.clock.Now().UTC())
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

type SendPrivateMessageRequest struct {
	SenderID    uint
	RecipientID uint
	Title       string
	Markdown    string
	ImageIDs    []uint
}

func (service *Service) SendPrivateMessage(ctx context.Context, request SendPrivateMessageRequest) (domain.Message, error) {
	if service == nil || service.messages == nil || service.identity == nil || service.organizations == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if len(request.ImageIDs) > 0 && service.files == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if request.SenderID == 0 || request.RecipientID == 0 || request.SenderID == request.RecipientID {
		return domain.Message{}, ErrRecipientUnavailable
	}
	sender, err := service.identity.LookupUser(ctx, request.SenderID)
	if err != nil || sender.ID != request.SenderID || !sender.Enabled {
		return domain.Message{}, ErrRecipientUnavailable
	}
	recipient, err := service.identity.LookupUser(ctx, request.RecipientID)
	if err != nil || recipient.ID != request.RecipientID || !recipient.Enabled {
		return domain.Message{}, ErrRecipientUnavailable
	}
	organizationID, err := service.sharedOrganization(ctx, request.SenderID, request.RecipientID)
	if err != nil {
		return domain.Message{}, err
	}
	compiled, err := domain.CompileMessageContent(request.Title, request.Markdown, service.contentLimits)
	if err != nil {
		return domain.Message{}, err
	}
	now := service.clock.Now().UTC()
	message := domain.Message{LogicalID: uuid.NewString(), OrganizationID: organizationID, SenderID: request.SenderID, Kind: domain.MessageKindPrivate, Status: domain.MessageStatusPublished, Title: compiled.Title, BodyHTML: compiled.HTML, PublishAt: &now, AggregateVersion: 1}
	event := domain.MessageEvent{EventID: uuid.NewString(), EventName: domain.EventNameMessageCreated, EventVersion: 1, OrganizationID: organizationID, OccurredAt: now, AggregateVersion: 1}
	persistence := MessagePersistence{Message: message, Recipient: &domain.PrivateRecipient{SenderID: request.SenderID, RecipientID: request.RecipientID}, Event: &event}
	if len(request.ImageIDs) == 0 {
		return service.messages.PersistMessage(ctx, persistence)
	}
	var persisted domain.Message
	if err := service.transactions.Run(ctx, func(tx context.Context) error {
		var err error
		persisted, err = service.messages.PersistMessage(tx, persistence)
		if err != nil {
			return err
		}
		return service.files.BindMessageImages(tx, MessageImageBinding{ActorID: request.SenderID, MessageLogicalID: persisted.LogicalID, ImageIDs: append([]uint(nil), request.ImageIDs...)})
	}); err != nil {
		return domain.Message{}, err
	}
	return persisted, nil
}

type RevokeMessageRequest struct {
	ActorID   uint
	MessageID uint
}

func (service *Service) RevokeOwnPrivateMessage(ctx context.Context, request RevokeMessageRequest) (domain.Message, error) {
	if service == nil || service.messages == nil {
		return domain.Message{}, ErrMessagingDependency
	}
	if request.ActorID == 0 || request.MessageID == 0 {
		return domain.Message{}, ErrRevokeNotAllowed
	}
	message, err := service.messages.FindMessage(ctx, request.MessageID)
	if err != nil {
		return domain.Message{}, err
	}
	if message.Kind != domain.MessageKindPrivate || message.SenderID != request.ActorID || message.Status != domain.MessageStatusPublished {
		return domain.Message{}, ErrRevokeNotAllowed
	}
	now := service.clock.Now().UTC()
	message.Status = domain.MessageStatusRevoked
	message.RevokedAt = &now
	message.AggregateVersion++
	event := domain.MessageEvent{EventID: uuid.NewString(), EventName: domain.EventNameMessageRevoked, EventVersion: 1, MessageCopyID: message.ID, OrganizationID: message.OrganizationID, OccurredAt: now, AggregateVersion: message.AggregateVersion}
	return service.messages.ChangeMessage(ctx, MessageChange{Message: message, Event: event})
}

func (service *Service) sharedOrganization(ctx context.Context, senderID, recipientID uint) (uint, error) {
	senderMemberships, err := service.organizations.Memberships(ctx, senderID)
	if err != nil {
		return 0, ErrRecipientUnavailable
	}
	recipientMemberships, err := service.organizations.Memberships(ctx, recipientID)
	if err != nil {
		return 0, ErrRecipientUnavailable
	}
	candidate := make(map[uint]struct{}, len(senderMemberships))
	for _, membership := range senderMemberships {
		if membership.OrganizationID != 0 {
			candidate[membership.OrganizationID] = struct{}{}
		}
	}
	shared := make([]uint, 0, len(candidate))
	for _, membership := range recipientMemberships {
		if _, ok := candidate[membership.OrganizationID]; ok {
			shared = append(shared, membership.OrganizationID)
		}
	}
	if len(shared) == 0 {
		return 0, ErrRecipientUnavailable
	}
	sort.Slice(shared, func(left, right int) bool { return shared[left] < shared[right] })
	return shared[0], nil
}
