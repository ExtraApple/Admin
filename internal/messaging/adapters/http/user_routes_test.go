package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type userHandlerMessageStoreFake struct {
	persisted application.MessagePersistence
	message   domain.Message
	changed   application.MessageChange
}

func (store *userHandlerMessageStoreFake) PersistMessage(_ context.Context, persistence application.MessagePersistence) (domain.Message, error) {
	store.persisted = persistence
	message := persistence.Message
	message.ID = 41
	store.message = message
	return message, nil
}
func (store *userHandlerMessageStoreFake) FindMessage(context.Context, uint) (domain.Message, error) {
	return store.message, nil
}
func (*userHandlerMessageStoreFake) ListMessages(context.Context, application.MessageListQuery) ([]domain.Message, int64, error) {
	panic("unused")
}
func (store *userHandlerMessageStoreFake) ChangeMessage(_ context.Context, change application.MessageChange) (domain.Message, error) {
	store.changed = change
	store.message = change.Message
	return change.Message, nil
}
func (*userHandlerMessageStoreFake) ListAudienceRules(context.Context, uint) ([]domain.AudienceRule, error) {
	panic("unused")
}

type userHandlerIdentityFake struct{}

func (userHandlerIdentityFake) LookupUser(_ context.Context, userID uint) (application.IdentityUser, error) {
	return application.IdentityUser{ID: userID, DisplayName: "User", Enabled: true}, nil
}
func (userHandlerIdentityFake) LookupVerifiedEmail(context.Context, uint) (application.VerifiedEmail, bool, error) {
	return application.VerifiedEmail{}, false, nil
}

type userHandlerOrganizationFake struct{}

func (userHandlerOrganizationFake) Memberships(context.Context, uint) ([]application.OrganizationMembership, error) {
	return []application.OrganizationMembership{{OrganizationID: 10}}, nil
}
func (userHandlerOrganizationFake) DescendantOrganizationIDs(context.Context, []uint) ([]uint, error) {
	panic("unused")
}
func (userHandlerOrganizationFake) MemberUserIDs(context.Context, uint) ([]uint, error) {
	panic("unused")
}
func (userHandlerOrganizationFake) RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error) {
	panic("unused")
}

type userHandlerInboxStoreFake struct {
	items       map[uint]application.InboxMessage
	query       application.InboxQuery
	readCalls   int
	deleteCalls int
}

func (store *userHandlerInboxStoreFake) ListInbox(_ context.Context, query application.InboxQuery) ([]application.InboxMessage, int64, error) {
	store.query = query
	items := make([]application.InboxMessage, 0, len(store.items))
	for _, item := range store.items {
		items = append(items, item)
	}
	return items, int64(len(items)), nil
}
func (store *userHandlerInboxStoreFake) FindInboxMessage(_ context.Context, identity application.InboxIdentity) (application.InboxMessage, bool, error) {
	item, ok := store.items[identity.UserID]
	return item, ok, nil
}
func (store *userHandlerInboxStoreFake) MarkInboxRead(context.Context, application.InboxIdentity, time.Time) (bool, error) {
	store.readCalls++
	return true, nil
}
func (store *userHandlerInboxStoreFake) DeletePrivateInbox(context.Context, application.InboxIdentity, time.Time) (bool, error) {
	store.deleteCalls++
	return true, nil
}
func (*userHandlerInboxStoreFake) CountUnreadInbox(context.Context, application.InboxQuery) (application.UnreadInboxCount, error) {
	return application.UnreadInboxCount{Private: 1, Total: 1}, nil
}

type userHandlerFilesFake struct {
	upload  application.MessageImageUpload
	binding application.MessageImageBinding
}

func (files *userHandlerFilesFake) UploadMessageImage(_ context.Context, upload application.MessageImageUpload) (application.TemporaryMessageImage, error) {
	files.upload = upload
	return application.TemporaryMessageImage{ID: 9, ExpiresAt: time.Date(2026, 8, 24, 1, 15, 0, 0, time.UTC)}, nil
}
func (files *userHandlerFilesFake) BindMessageImages(_ context.Context, binding application.MessageImageBinding) error {
	files.binding = binding
	return nil
}
func (*userHandlerFilesFake) OpenMessageImage(context.Context, application.VisibleMessageImage) (application.MessageImageContent, error) {
	return application.MessageImageContent{Reader: io.NopCloser(strings.NewReader("img")), ContentType: "image/png", Size: 3}, nil
}

func TestUserMessageHandlerSendsPrivateMessageWithAuthenticatedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &userHandlerMessageStoreFake{}
	files := &userHandlerFilesFake{}
	service := application.NewService(application.Dependencies{Messages: store, Identity: userHandlerIdentityFake{}, Organizations: userHandlerOrganizationFake{}, Files: files, Clock: application.ClockFunc(func() time.Time { return time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC) })})
	descriptor := findMessageRoute(t, Routes(service), http.MethodPost, "/api/user/messages/private")
	context, response := newGinRequest(t, http.MethodPost, "/api/user/messages/private", `{"recipient_id":8,"title":"hello","markdown":"body","image_ids":[91]}`)
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	if response.Code != http.StatusOK || store.persisted.Message.SenderID != 7 || store.persisted.Recipient == nil || store.persisted.Recipient.RecipientID != 8 || store.persisted.Message.BodyHTML == "body" {
		t.Fatalf("send response=%d body=%s persistence=%#v", response.Code, response.Body.String(), store.persisted)
	}
	if files.binding.ActorID != 7 || files.binding.MessageLogicalID != store.message.LogicalID || len(files.binding.ImageIDs) != 1 || files.binding.ImageIDs[0] != 91 {
		t.Fatalf("image binding=%#v", files.binding)
	}
}

func TestUserMessageHandlerMapsInvalidJSONToFourFieldError(t *testing.T) {
	service := application.NewService(application.Dependencies{})
	descriptor := findMessageRoute(t, Routes(service), http.MethodPost, "/api/user/messages/private")
	context, response := newGinRequest(t, http.MethodPost, "/api/user/messages/private", `{invalid`)
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusBadRequest || envelope["error_code"] != CodeRequestInvalid || envelope["code"] != float64(http.StatusBadRequest) || envelope["data"] != nil {
		t.Fatalf("invalid request response=%d body=%s envelope=%#v err=%v", response.Code, response.Body.String(), envelope, err)
	}
}

func TestUserMessageHandlerListsFiltersAndMutatesInbox(t *testing.T) {
	item := application.InboxMessage{Message: domain.Message{ID: 41, Kind: domain.MessageKindAnnouncement, Title: "notice"}}
	inbox := &userHandlerInboxStoreFake{items: map[uint]application.InboxMessage{7: item}}
	service := application.NewService(application.Dependencies{Inbox: inbox, Organizations: userHandlerOrganizationFake{}, Clock: application.ClockFunc(time.Now)})
	list := findMessageRoute(t, Routes(service), http.MethodGet, "/api/user/messages")
	context, response := newGinRequest(t, http.MethodGet, "/api/user/messages?page=2&size=5&kind=announcement&read=false&category_id=3&keyword=notice", "")
	context.Set("userID", uint(7))
	list.Handler(context)
	if response.Code != http.StatusOK || inbox.query.UserID != 7 || inbox.query.Offset != 5 || inbox.query.Limit != 5 || inbox.query.CategoryID != 3 || inbox.query.Keyword != "notice" || len(inbox.query.Kinds) != 1 || inbox.query.Kinds[0] != domain.MessageKindAnnouncement || inbox.query.Read == nil || *inbox.query.Read {
		t.Fatalf("list response=%d query=%#v body=%s", response.Code, inbox.query, response.Body.String())
	}

	read := findMessageRoute(t, Routes(service), http.MethodPut, "/api/user/messages/:id/read")
	context, response = newGinRequest(t, http.MethodPut, "/api/user/messages/41/read", "")
	context.Params = gin.Params{{Key: "id", Value: "41"}}
	context.Set("userID", uint(7))
	read.Handler(context)
	if response.Code != http.StatusOK || inbox.readCalls != 1 {
		t.Fatalf("read response=%d calls=%d body=%s", response.Code, inbox.readCalls, response.Body.String())
	}

	remove := findMessageRoute(t, Routes(service), http.MethodDelete, "/api/user/messages/:id/inbox")
	context, response = newGinRequest(t, http.MethodDelete, "/api/user/messages/41/inbox", "")
	context.Params = gin.Params{{Key: "id", Value: "41"}}
	context.Set("userID", uint(7))
	remove.Handler(context)
	if response.Code != http.StatusOK || inbox.deleteCalls != 1 {
		t.Fatalf("delete response=%d calls=%d body=%s", response.Code, inbox.deleteCalls, response.Body.String())
	}
}

func TestUserMessageHandlerReadsVisibleImageThroughMessagingService(t *testing.T) {
	message := domain.Message{ID: 41, LogicalID: "logical-41", Status: domain.MessageStatusPublished}
	inbox := &userHandlerInboxStoreFake{items: map[uint]application.InboxMessage{7: {Message: message}}}
	files := &userHandlerFilesFake{}
	service := application.NewService(application.Dependencies{Inbox: inbox, Organizations: userHandlerOrganizationFake{}, Files: files})
	descriptor := findMessageRoute(t, Routes(service), http.MethodGet, "/api/user/messages/:id/images/:image_id")
	context, response := newGinRequest(t, http.MethodGet, "/api/user/messages/41/images/9", "")
	context.Params = gin.Params{{Key: "id", Value: "41"}, {Key: "image_id", Value: "9"}}
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	if response.Code != http.StatusOK || response.Body.String() != "img" || response.Header().Get("Content-Type") != "image/png" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("image response=%d headers=%#v body=%q", response.Code, response.Header(), response.Body.String())
	}
}

func TestUserMessageHandlerUploadsDedicatedMessageImage(t *testing.T) {
	files := &userHandlerFilesFake{}
	service := application.NewService(application.Dependencies{Files: files})
	descriptor := findMessageRoute(t, Routes(service), http.MethodPost, "/api/user/messages/images")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "notice.png")
	if err != nil {
		t.Fatalf("CreateFormFile() = %v", err)
	}
	_, _ = part.Write([]byte("image"))
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart Close() = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/user/messages/images", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	context.Set("userID", uint(7))
	descriptor.Handler(context)
	if response.Code != http.StatusOK || files.upload.UploaderID != 7 || files.upload.FileName != "notice.png" || files.upload.ContentType != "application/octet-stream" || files.upload.Size != 5 {
	}
}

func findMessageRoute(t *testing.T, routes []routecatalog.Descriptor, method, path string) routecatalog.Descriptor {
	t.Helper()
	for _, route := range routes {
		if route.Method == method && route.Path == path {
			return route
		}
	}
	t.Fatalf("route %s %s not found", method, path)
	return routecatalog.Descriptor{}
}

func newGinRequest(t *testing.T, method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request
	return context, response
}
