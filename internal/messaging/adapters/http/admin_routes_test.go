package httpadapter

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type adminMessageStoreFake struct {
	message   domain.Message
	persisted application.MessagePersistence
	changed   application.MessageChange
	listQuery application.MessageListQuery
}

func (store *adminMessageStoreFake) PersistMessage(_ context.Context, persistence application.MessagePersistence) (domain.Message, error) {
	store.persisted = persistence
	message := persistence.Message
	message.ID = 41
	store.message = message
	return message, nil
}
func (store *adminMessageStoreFake) FindMessage(context.Context, uint) (domain.Message, error) {
	if store.message.ID == 0 {
		return domain.Message{}, application.ErrNotFound
	}
	return store.message, nil
}
func (store *adminMessageStoreFake) ListMessages(_ context.Context, query application.MessageListQuery) ([]domain.Message, int64, error) {
	store.listQuery = query
	if store.message.ID == 0 {
		return nil, 0, nil
	}
	return []domain.Message{store.message}, 1, nil
}
func (store *adminMessageStoreFake) ChangeMessage(_ context.Context, change application.MessageChange) (domain.Message, error) {
	store.changed = change
	store.message = change.Message
	return change.Message, nil
}
func (*adminMessageStoreFake) ListAudienceRules(context.Context, uint) ([]domain.AudienceRule, error) {
	return []domain.AudienceRule{{OrganizationID: 10, Type: domain.AudienceTypeOrganization}}, nil
}

type adminCategoryStoreFake struct {
	category domain.MessageCategory
	list     []domain.MessageCategory
}

func (store *adminCategoryStoreFake) CreateCategory(_ context.Context, category domain.MessageCategory) (domain.MessageCategory, error) {
	category.ID = 3
	store.category = category
	return category, nil
}
func (store *adminCategoryStoreFake) FindCategory(context.Context, uint) (domain.MessageCategory, error) {
	if store.category.ID == 0 {
		return domain.MessageCategory{}, application.ErrNotFound
	}
	return store.category, nil
}
func (store *adminCategoryStoreFake) FindCategoryByCode(context.Context, uint, string) (domain.MessageCategory, error) {
	if store.category.ID == 0 {
		return domain.MessageCategory{}, application.ErrNotFound
	}
	return store.category, nil
}
func (store *adminCategoryStoreFake) ListCategories(context.Context, application.CategoryListQuery) ([]domain.MessageCategory, int64, error) {
	return store.list, int64(len(store.list)), nil
}
func (store *adminCategoryStoreFake) UpdateCategory(_ context.Context, category domain.MessageCategory) error {
	store.category = category
	return nil
}
func (store *adminCategoryStoreFake) DeleteCategoryIfUnused(context.Context, uint) (bool, error) {
	return true, nil
}

type adminAuthorizationFake struct {
	permissions []string
	scope       application.MessageOrganizationScope
}

func (authorization *adminAuthorizationFake) HasPermission(_ context.Context, _ uint, permission string) (bool, error) {
	authorization.permissions = append(authorization.permissions, permission)
	return true, nil
}
func (authorization *adminAuthorizationFake) OrganizationScope(context.Context, uint) (application.MessageOrganizationScope, error) {
	return authorization.scope, nil
}

type adminOrganizationFake struct{}

func (adminOrganizationFake) Memberships(context.Context, uint) ([]application.OrganizationMembership, error) {
	return []application.OrganizationMembership{{OrganizationID: 10}}, nil
}
func (adminOrganizationFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	return []uint{10}, nil
}
func (adminOrganizationFake) MemberUserIDs(context.Context, uint) ([]uint, error) {
	return []uint{7}, nil
}
func (adminOrganizationFake) RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error) {
	return []uint{7}, nil
}
func (adminOrganizationFake) AllOrganizationIDs(context.Context) ([]uint, error) {
	return []uint{10}, nil
}

type adminOutboxStoreFake struct {
	replayID uint
}

func (*adminOutboxStoreFake) ClaimOutbox(context.Context, application.OutboxClaim) ([]domain.MessageOutbox, error) {
	return nil, nil
}
func (*adminOutboxStoreFake) RenewOutboxLease(context.Context, application.OutboxLease) (bool, error) {
	return true, nil
}
func (*adminOutboxStoreFake) MarkOutboxPublished(context.Context, application.OutboxLease) (bool, error) {
	return true, nil
}
func (*adminOutboxStoreFake) RecordOutboxFailure(context.Context, application.OutboxFailure) (bool, error) {
	return true, nil
}
func (store *adminOutboxStoreFake) ReplayOutbox(_ context.Context, id uint) (bool, error) {
	store.replayID = id
	return true, nil
}
func (*adminOutboxStoreFake) ListOutboxes(context.Context, application.OutboxListQuery) ([]domain.MessageOutbox, int64, error) {
	return []domain.MessageOutbox{{ID: 4, Status: domain.OutboxStatusDead}}, 1, nil
}

type adminDeadLetterStoreFake struct {
	discardID    uint
	invalidClaim bool
}

func (*adminDeadLetterStoreFake) RecordConsumerDeadLetter(context.Context, application.ConsumerDeadLetterInput) (application.ConsumerDeadLetter, error) {
	return application.ConsumerDeadLetter{}, nil
}
func (*adminDeadLetterStoreFake) ListConsumerDeadLetters(context.Context, application.ConsumerDeadLetterListQuery) ([]application.ConsumerDeadLetter, int64, error) {
	return []application.ConsumerDeadLetter{{ID: 8, ConsumerName: "websocket", Event: domain.MessageEvent{EventID: "event-8"}, Status: domain.ConsumerDLQStatusPending}}, 1, nil
}
func (store *adminDeadLetterStoreFake) ClaimConsumerDeadLetterReplay(context.Context, application.ConsumerDeadLetterReplayClaim) (application.ConsumerDeadLetter, bool, error) {
	if store.invalidClaim {
		return application.ConsumerDeadLetter{ID: 8, ConsumerName: "websocket", Event: domain.MessageEvent{EventID: "invalid:fingerprint"}, Status: domain.ConsumerDLQStatusReplaying, ReplayLeaseFence: 1, Invalid: true, Fingerprint: "fingerprint", Replayable: false}, true, nil
	}
	return application.ConsumerDeadLetter{ID: 8, ConsumerName: "websocket", Event: domain.MessageEvent{EventID: "event-8"}, Status: domain.ConsumerDLQStatusPending, ReplayLeaseFence: 1}, true, nil
}
func (*adminDeadLetterStoreFake) MarkConsumerDeadLetterReplayed(context.Context, application.ConsumerDeadLetterReplayResult) (bool, error) {
	return true, nil
}
func (*adminDeadLetterStoreFake) ReturnConsumerDeadLetterPending(context.Context, application.ConsumerDeadLetterReplayResult) (bool, error) {
	return true, nil
}
func (store *adminDeadLetterStoreFake) DiscardConsumerDeadLetter(_ context.Context, id uint, _ time.Time) (bool, error) {
	store.discardID = id
	return true, nil
}
func (*adminDeadLetterStoreFake) CleanupFinalConsumerDeadLetters(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

type adminReplayPublisherFake struct{}

func (adminReplayPublisherFake) PublishConsumerReplay(context.Context, string, domain.MessageEvent) error {
	return nil
}

func newAdminService(messageStore *adminMessageStoreFake, categoryStore *adminCategoryStoreFake, outboxStore application.OutboxStore, deadLetterStore application.ConsumerDeadLetterStore) *application.Service {
	authorization := &adminAuthorizationFake{scope: application.MessageOrganizationScope{All: true}}
	return application.NewService(application.Dependencies{
		Messages: messageStore, Categories: categoryStore, Outboxes: outboxStore, ConsumerDeadLetters: deadLetterStore,
		ConsumerDeadLetterReplay: application.NewConsumerDeadLetterReplayService(application.ConsumerDeadLetterReplayConfig{Store: deadLetterStore, Publisher: adminReplayPublisherFake{}, WorkerID: "admin-worker"}),
		Authorization:            authorization, Organizations: adminOrganizationFake{}, Transactions: adminDirectTransactionRunner{}, Clock: application.ClockFunc(time.Now),
	})
}

type adminDirectTransactionRunner struct{}

func (adminDirectTransactionRunner) Run(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestAdminRoutesDeclareManagementSurfaceWithPermissionCodes(t *testing.T) {
	routes := AdminRoutes(nil)
	want := map[string]string{
		"GET /api/admin/messages":                         "admin.messages.broadcast.manage",
		"POST /api/admin/messages/broadcast":              "admin.messages.broadcast.manage",
		"POST /api/admin/messages/:id/revoke":             "admin.messages.revoke.all",
		"GET /api/admin/announcements":                    "admin.messages.announcement.manage",
		"POST /api/admin/announcements":                   "admin.messages.announcement.manage",
		"GET /api/admin/announcements/:id":                "admin.messages.announcement.manage",
		"PUT /api/admin/announcements/:id":                "admin.messages.announcement.manage",
		"POST /api/admin/announcements/:id/publish":       "admin.messages.announcement.manage",
		"POST /api/admin/announcements/:id/revoke":        "admin.messages.revoke.all",
		"GET /api/admin/message-categories":               "admin.messages.category.manage",
		"POST /api/admin/message-categories":              "admin.messages.category.manage",
		"PUT /api/admin/message-categories/:id":           "admin.messages.category.manage",
		"DELETE /api/admin/message-categories/:id":        "admin.messages.category.manage",
		"POST /api/admin/messages/images":                 "admin.messages.announcement.manage",
		"GET /api/admin/message-outboxes":                 "admin.messages.outbox.replay",
		"POST /api/admin/message-outboxes/:id/replay":     "admin.messages.outbox.replay",
		"GET /api/admin/message-dead-letters":             "admin.messages.dead-letter.manage",
		"POST /api/admin/message-dead-letters/:id/replay": "admin.messages.dead-letter.manage",
		"DELETE /api/admin/message-dead-letters/:id":      "admin.messages.dead-letter.manage",
	}
	if len(routes) != len(want) {
		t.Fatalf("admin route count=%d want=%d", len(routes), len(want))
	}
	for _, route := range routes {
		if want[route.Method+" "+route.Path] != route.DefaultPermissionCode || route.Access != routecatalog.PermissionControlled {
			t.Fatalf("route %s %s access=%d permission=%q", route.Method, route.Path, route.Access, route.DefaultPermissionCode)
		}
	}
}

func TestAdminBroadcastHandlerUsesAuthenticatedActorAndReturnsMessageDTO(t *testing.T) {
	messages := &adminMessageStoreFake{}
	categories := &adminCategoryStoreFake{category: domain.MessageCategory{ID: 3, OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}}
	service := newAdminService(messages, categories, &adminOutboxStoreFake{}, &adminDeadLetterStoreFake{})
	route := findMessageRoute(t, AdminRoutes(service), http.MethodPost, "/api/admin/messages/broadcast")
	context, response := newGinRequest(t, http.MethodPost, "/api/admin/messages/broadcast", `{"category_code":"notice","title":"Release","markdown":"Body","targets":[{"organization_id":10,"type":"organization"}]}`)
	context.Set("userID", uint(7))
	route.Handler(context)
	if response.Code != http.StatusOK || messages.persisted.Message.SenderID != 7 || messages.persisted.Message.Kind != domain.MessageKindBroadcast || messages.persisted.Message.BodyHTML == "Body" {
		t.Fatalf("broadcast response=%d body=%s persistence=%#v", response.Code, response.Body.String(), messages.persisted)
	}
}

func TestAdminBroadcastHandlerPassesImageIDsToMessageOperation(t *testing.T) {
	messages := &adminMessageStoreFake{}
	categories := &adminCategoryStoreFake{category: domain.MessageCategory{ID: 3, OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}}
	files := &userHandlerFilesFake{}
	service := application.NewService(application.Dependencies{
		Messages: messages, Categories: categories, Files: files,
		Authorization: &adminAuthorizationFake{scope: application.MessageOrganizationScope{All: true}},
		Organizations: adminOrganizationFake{}, Transactions: adminDirectTransactionRunner{}, Clock: application.ClockFunc(time.Now),
	})
	route := findMessageRoute(t, AdminRoutes(service), http.MethodPost, "/api/admin/messages/broadcast")
	context, response := newGinRequest(t, http.MethodPost, "/api/admin/messages/broadcast", `{"category_code":"notice","title":"Release","markdown":"Body","targets":[{"organization_id":10,"type":"organization"}],"image_ids":[91]}`)
	context.Set("userID", uint(7))
	route.Handler(context)
	if response.Code != http.StatusOK || files.binding.ActorID != 7 || files.binding.MessageLogicalID != messages.message.LogicalID || len(files.binding.ImageIDs) != 1 || files.binding.ImageIDs[0] != 91 {
		t.Fatalf("response=%d body=%s binding=%#v", response.Code, response.Body.String(), files.binding)
	}
}

func TestAdminAnnouncementHandlersPassImageIDsToMessageOperations(t *testing.T) {
	newService := func(messages *adminMessageStoreFake, files *userHandlerFilesFake) *application.Service {
		return application.NewService(application.Dependencies{
			Messages: messages, Categories: &adminCategoryStoreFake{category: domain.MessageCategory{ID: 3, OrganizationID: 10, Code: "notice", Name: "Notice", Enabled: true}}, Files: files,
			Authorization: &adminAuthorizationFake{scope: application.MessageOrganizationScope{All: true}},
			Organizations: adminOrganizationFake{}, Transactions: adminDirectTransactionRunner{}, Clock: application.ClockFunc(func() time.Time { return time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC) }),
		})
	}

	createMessages := &adminMessageStoreFake{}
	createFiles := &userHandlerFilesFake{}
	create := findMessageRoute(t, AdminRoutes(newService(createMessages, createFiles)), http.MethodPost, "/api/admin/announcements")
	context, response := newGinRequest(t, http.MethodPost, "/api/admin/announcements", `{"category_code":"notice","title":"Notice","markdown":"Body","targets":[{"organization_id":10,"type":"organization"}],"image_ids":[92]}`)
	context.Set("userID", uint(7))
	create.Handler(context)
	if response.Code != http.StatusOK || createFiles.binding.ActorID != 7 || createFiles.binding.MessageLogicalID != createMessages.message.LogicalID || len(createFiles.binding.ImageIDs) != 1 || createFiles.binding.ImageIDs[0] != 92 {
		t.Fatalf("create response=%d body=%s binding=%#v", response.Code, response.Body.String(), createFiles.binding)
	}

	editMessages := &adminMessageStoreFake{message: domain.Message{ID: 41, LogicalID: "logical-41", OrganizationID: 10, CategoryID: 3, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusPublished, Title: "Old", BodyHTML: "<p>Old</p>"}}
	editFiles := &userHandlerFilesFake{}
	edit := findMessageRoute(t, AdminRoutes(newService(editMessages, editFiles)), http.MethodPut, "/api/admin/announcements/:id")
	context, response = newGinRequest(t, http.MethodPut, "/api/admin/announcements/41", `{"title":"Edited","markdown":"Body","targets":[{"organization_id":10,"type":"organization"}],"image_ids":[93]}`)
	context.Params = gin.Params{{Key: "id", Value: "41"}}
	context.Set("userID", uint(7))
	edit.Handler(context)
	if response.Code != http.StatusOK || editFiles.binding.ActorID != 7 || editFiles.binding.MessageLogicalID != editMessages.message.LogicalID || len(editFiles.binding.ImageIDs) != 1 || editFiles.binding.ImageIDs[0] != 93 {
		t.Fatalf("edit response=%d body=%s binding=%#v", response.Code, response.Body.String(), editFiles.binding)
	}

	publishMessages := &adminMessageStoreFake{message: domain.Message{ID: 41, LogicalID: "logical-41", OrganizationID: 10, CategoryID: 3, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusDraft, Title: "Draft", BodyHTML: "<p>Draft</p>"}}
	publishFiles := &userHandlerFilesFake{}
	publish := findMessageRoute(t, AdminRoutes(newService(publishMessages, publishFiles)), http.MethodPost, "/api/admin/announcements/:id/publish")
	context, response = newGinRequest(t, http.MethodPost, "/api/admin/announcements/41/publish", `{"image_ids":[94]}`)
	context.Params = gin.Params{{Key: "id", Value: "41"}}
	context.Set("userID", uint(7))
	publish.Handler(context)
	if response.Code != http.StatusOK || publishFiles.binding.ActorID != 7 || publishFiles.binding.MessageLogicalID != publishMessages.message.LogicalID || len(publishFiles.binding.ImageIDs) != 1 || publishFiles.binding.ImageIDs[0] != 94 {
		t.Fatalf("publish response=%d body=%s binding=%#v", response.Code, response.Body.String(), publishFiles.binding)
	}
}

func TestAdminOutboxAndDeadLetterHandlersExposeControlledMetadataAndMutateOnlyByID(t *testing.T) {
	messages := &adminMessageStoreFake{}
	categories := &adminCategoryStoreFake{}
	outboxes := &adminOutboxStoreFake{}
	deadLetters := &adminDeadLetterStoreFake{}
	service := newAdminService(messages, categories, outboxes, deadLetters)
	listOutbox := findMessageRoute(t, AdminRoutes(service), http.MethodGet, "/api/admin/message-outboxes")
	context, response := newGinRequest(t, http.MethodGet, "/api/admin/message-outboxes?status=dead", "")
	context.Set("userID", uint(1))
	listOutbox.Handler(context)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "password") {
		t.Fatalf("outbox response=%d body=%s", response.Code, response.Body.String())
	}
	replayOutbox := findMessageRoute(t, AdminRoutes(service), http.MethodPost, "/api/admin/message-outboxes/:id/replay")
	context, response = newGinRequest(t, http.MethodPost, "/api/admin/message-outboxes/4/replay", "")
	context.Params = gin.Params{{Key: "id", Value: "4"}}
	context.Set("userID", uint(1))
	replayOutbox.Handler(context)
	if response.Code != http.StatusOK || outboxes.replayID != 4 {
		t.Fatalf("outbox replay response=%d body=%s id=%d", response.Code, response.Body.String(), outboxes.replayID)
	}
	discard := findMessageRoute(t, AdminRoutes(service), http.MethodDelete, "/api/admin/message-dead-letters/:id")
	context, response = newGinRequest(t, http.MethodDelete, "/api/admin/message-dead-letters/8", "")
	context.Params = gin.Params{{Key: "id", Value: "8"}}
	context.Set("userID", uint(1))
	discard.Handler(context)
	if response.Code != http.StatusOK || deadLetters.discardID != 8 {
		t.Fatalf("dead letter discard response=%d body=%s id=%d", response.Code, response.Body.String(), deadLetters.discardID)
	}
}

func TestAdminDeadLetterHandlerRejectsReplayOfInvalidProjection(t *testing.T) {
	deadLetters := &adminDeadLetterStoreFake{invalidClaim: true}
	service := newAdminService(&adminMessageStoreFake{}, &adminCategoryStoreFake{}, &adminOutboxStoreFake{}, deadLetters)
	replay := findMessageRoute(t, AdminRoutes(service), http.MethodPost, "/api/admin/message-dead-letters/:id/replay")
	context, response := newGinRequest(t, http.MethodPost, "/api/admin/message-dead-letters/8/replay", "")
	context.Params = gin.Params{{Key: "id", Value: "8"}}
	context.Set("userID", uint(1))
	replay.Handler(context)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "MSG_STATE_CONFLICT") {
		t.Fatalf("invalid dead-letter replay response=%d body=%s", response.Code, response.Body.String())
	}
}
