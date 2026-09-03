package httpadapter

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"admin/internal/uploadsecurity"
	websocket "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

const (
	messageMultipartOverheadBytes int64 = 1024 * 1024
	messageMultipartMemoryBytes   int64 = 1024 * 1024
	defaultMessagePageSize              = 20
	maxMessagePageSize                  = 100
)

type handler struct {
	service *application.Service
}

func Routes(service *application.Service) []routecatalog.Descriptor {
	h := &handler{service: service}
	return []routecatalog.Descriptor{
		messageRoute(http.MethodPost, "/api/user/messages/private", "Send Private Message", routecatalog.Authenticated, h.sendPrivate, SendPrivateMessageRequest{}, MessageDTO{}),
		messageRoute(http.MethodGet, "/api/user/messages", "List User Messages", routecatalog.Authenticated, h.listInbox, nil, InboxResponse{}),
		messageRoute(http.MethodGet, "/api/user/messages/unread-count", "Count Unread Messages", routecatalog.Authenticated, h.unreadCount, nil, UnreadCountResponse{}),
		messageRoute(http.MethodGet, "/api/user/messages/:id", "Get User Message", routecatalog.Authenticated, h.getMessage, nil, MessageDTO{}),
		messageRoute(http.MethodPut, "/api/user/messages/:id/read", "Mark Message Read", routecatalog.Authenticated, h.markRead, nil, nil),
		messageRoute(http.MethodDelete, "/api/user/messages/:id/inbox", "Delete Private Inbox", routecatalog.Authenticated, h.deleteInbox, nil, nil),
		messageRoute(http.MethodPost, "/api/user/messages/:id/revoke", "Revoke Private Message", routecatalog.Authenticated, h.revokePrivate, nil, MessageDTO{}),
		messageImageRoute(http.MethodPost, "/api/user/messages/images", "Upload User Message Image", routecatalog.Authenticated, h.uploadImage),
		messageImageReadRoute(http.MethodGet, "/api/user/messages/:id/images/:image_id", "Read User Message Image", routecatalog.Authenticated, h.readImage),
	}
}

// WebSocketRoutes declares the authenticated ticket and native protocol upgrade
func WebSocketRoutes(tickets *application.WebSocketTicketService, gateway *application.WebSocketGateway) []routecatalog.Descriptor {
	return webSocketRoutes(tickets, gateway, acceptCoderWebSocket)
}

func webSocketRoutes(tickets *application.WebSocketTicketService, gateway *application.WebSocketGateway, accept func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error)) []routecatalog.Descriptor {
	h := &webSocketHandler{tickets: tickets, gateway: gateway, accept: accept}
	return []routecatalog.Descriptor{
		messageRoute(http.MethodPost, "/api/user/messages/ws-ticket", "Issue Message WebSocket Ticket", routecatalog.Authenticated, h.issueTicket, nil, WebSocketTicketResponse{}),
		webSocketUpgradeRoute(http.MethodGet, "/api/user/messages/ws", "Upgrade Message WebSocket", routecatalog.Authenticated, h.upgrade),
	}
}

func webSocketUpgradeRoute(method, path, name string, access routecatalog.AccessLevel, handler gin.HandlerFunc) routecatalog.Descriptor {
	descriptor := messageRoute(method, path, name, access, handler, nil, nil)
	responses := messageResponses(nil)
	delete(responses, http.StatusOK)
	responses[http.StatusSwitchingProtocols] = routecatalog.Response{Description: "native RFC 6455 WebSocket protocol", Kind: routecatalog.NoBody}
	descriptor.OpenAPI.Protocol = "websocket"
	descriptor.OpenAPI.Description = "Native RFC 6455 WebSocket upgrade; successful connections do not use the business JSON envelope."
	descriptor.OpenAPI.Responses = responses
	return descriptor
}

type webSocketHandler struct {
	tickets *application.WebSocketTicketService
	gateway *application.WebSocketGateway
	accept  func(http.ResponseWriter, *http.Request) (application.WebSocketConnection, error)
}

func (h *webSocketHandler) issueTicket(c *gin.Context) {
	if h == nil || h.tickets == nil {
		writeMessageError(c, application.ErrMessagingDependency)
		return
	}
	ticket, err := h.tickets.Issue(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, WebSocketTicketResponse{Ticket: ticket, ExpiresIn: int(application.WebSocketTicketTTL / time.Second)})
}

func (h *webSocketHandler) upgrade(c *gin.Context) {
	if h == nil || h.gateway == nil {
		writeMessageError(c, application.ErrWebSocketGatewayUnavailable)
		return
	}
	if h.accept == nil {
		writeMessageError(c, application.ErrWebSocketGatewayUnavailable)
		return
	}
	request := application.WebSocketRequest{Ticket: c.Query("ticket"), Cursor: c.Query("cursor")}
	if err := h.gateway.Connect(c.Request.Context(), request, c.GetUint("userID"), func(context.Context) (application.WebSocketConnection, error) {
		return h.accept(c.Writer, c.Request)
	}); err != nil {
		writeMessageError(c, err)
	}
}

type coderWebSocketConnection struct{ connection *websocket.Conn }

func (connection coderWebSocketConnection) Read(ctx context.Context) ([]byte, error) {
	_, payload, err := connection.connection.Read(ctx)
	return payload, err
}

func (connection coderWebSocketConnection) Write(ctx context.Context, payload []byte) error {
	return connection.connection.Write(ctx, websocket.MessageText, payload)
}

func (connection coderWebSocketConnection) Close(reason string) error {
	return connection.connection.Close(websocket.StatusGoingAway, reason)
}

func acceptCoderWebSocket(writer http.ResponseWriter, request *http.Request) (application.WebSocketConnection, error) {
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return nil, err
	}
	return coderWebSocketConnection{connection: connection}, nil
}

func messageRoute(method, path, name string, access routecatalog.AccessLevel, handler gin.HandlerFunc, request, response any) routecatalog.Descriptor {
	requestBody := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if request != nil {
		requestBody = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(request), Required: true}
	}
	return routecatalog.Descriptor{
		Method: method, Path: path, Access: access, Handler: handler,
		Name: name, Group: "messaging", DefaultAuditCategory: "message",
		OpenAPI: routecatalog.Operation{Summary: name, Request: requestBody, Responses: messageResponses(response)},
	}
}

func messageImageReadRoute(method, path, name string, access routecatalog.AccessLevel, handler gin.HandlerFunc) routecatalog.Descriptor {
	descriptor := messageRoute(method, path, name, access, handler, nil, nil)
	responses := messageResponses(nil)
	responses[http.StatusOK] = routecatalog.Response{Description: "validated message image", Kind: routecatalog.BinaryBody, ContentTypes: []string{"image/jpeg", "image/png", "image/webp"}}
	descriptor.OpenAPI.Responses = responses
	return descriptor
}

func messageImageRoute(method, path, name string, access routecatalog.AccessLevel, handler gin.HandlerFunc) routecatalog.Descriptor {
	descriptor := messageRoute(method, path, name, access, handler, nil, MessageImageUploadResponse{})
	descriptor.OpenAPI.Request = routecatalog.RequestBody{Kind: routecatalog.MultipartBody, Required: true, FileField: "file", FileDescription: "Validated JPEG, PNG or WebP message image"}
	return descriptor
}

func messageResponses(response any) map[int]routecatalog.Response {
	responses := map[int]routecatalog.Response{http.StatusOK: routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(response))}
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity, http.StatusServiceUnavailable, http.StatusInternalServerError} {
		definitions := messageErrorDefinitionsForStatus(status)
		if len(definitions) != 0 {
			responses[status] = routecatalog.ErrorResponse("message operation failed", definitions...)
		}
	}
	return responses
}

func messageErrorDefinitionsForStatus(status int) []httpresponse.ErrorDefinition {
	all := MessageErrorDefinitions()
	result := make([]httpresponse.ErrorDefinition, 0)
	for _, definition := range all {
		if definition.Status == status {
			result = append(result, definition)
		}
	}
	return result
}

func (h *handler) sendPrivate(c *gin.Context) {
	var request SendPrivateMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	if h.service == nil {
		writeMessageError(c, application.ErrMessagingDependency)
		return
	}
	message, err := h.service.SendPrivateMessage(c.Request.Context(), application.SendPrivateMessageRequest{SenderID: c.GetUint("userID"), RecipientID: request.RecipientID, Title: request.Title, Markdown: request.Markdown, ImageIDs: append([]uint(nil), request.ImageIDs...)})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toMessageDTO(application.InboxMessage{Message: message}))
}

func (h *handler) listInbox(c *gin.Context) {
	if h.service == nil {
		writeMessageError(c, application.ErrMessagingDependency)
		return
	}
	request, err := parseInboxQuery(c, c.GetUint("userID"))
	if err != nil {
		writeMessageError(c, err)
		return
	}
	result, err := h.service.ListInbox(c.Request.Context(), request)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, InboxResponse{Items: toMessageDTOs(result.Items), Total: result.Total, Offset: request.Offset, Limit: request.Limit})
}

func (h *handler) unreadCount(c *gin.Context) {
	if h.service == nil {
		writeMessageError(c, application.ErrMessagingDependency)
		return
	}
	counts, err := h.service.CountUnreadInbox(c.Request.Context(), application.UnreadInboxRequest{UserID: c.GetUint("userID")})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, UnreadCountResponse{Private: counts.Private, Broadcast: counts.Broadcast, Announcement: counts.Announcement, Total: counts.Total})
}

func (h *handler) getMessage(c *gin.Context) {
	messageID, ok := messagePathID(c)
	if !ok || h.service == nil {
		if h.service == nil {
			writeMessageError(c, application.ErrMessagingDependency)
		}
		return
	}
	item, err := h.service.GetInboxMessage(c.Request.Context(), application.InboxIdentityRequest{UserID: c.GetUint("userID"), MessageID: messageID})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toMessageDTO(item))
}

func (h *handler) markRead(c *gin.Context) {
	h.markInboxMutation(c, func(request application.InboxIdentityRequest) error {
		return h.service.MarkInboxRead(c.Request.Context(), request)
	})
}

func (h *handler) deleteInbox(c *gin.Context) {
	h.markInboxMutation(c, func(request application.InboxIdentityRequest) error {
		return h.service.DeletePrivateInbox(c.Request.Context(), request)
	})
}

func (h *handler) markInboxMutation(c *gin.Context, operation func(application.InboxIdentityRequest) error) {
	messageID, ok := messagePathID(c)
	if !ok || h.service == nil {
		if h.service == nil {
			writeMessageError(c, application.ErrMessagingDependency)
		}
		return
	}
	if err := operation(application.InboxIdentityRequest{UserID: c.GetUint("userID"), MessageID: messageID}); err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, nil)
}

func (h *handler) revokePrivate(c *gin.Context) {
	messageID, ok := messagePathID(c)
	if !ok || h.service == nil {
		if h.service == nil {
			writeMessageError(c, application.ErrMessagingDependency)
		}
		return
	}
	message, err := h.service.RevokeOwnPrivateMessage(c.Request.Context(), application.RevokeMessageRequest{ActorID: c.GetUint("userID"), MessageID: messageID})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, toMessageDTO(application.InboxMessage{Message: message}))
}

func (h *handler) uploadImage(c *gin.Context) {
	file, cleanup, err := parseMessageImage(c)
	defer cleanup()
	if err != nil {
		writeMessageError(c, err)
		return
	}
	reader, err := file.Open()
	if err != nil {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	defer reader.Close()
	if h.service == nil {
		writeMessageError(c, application.ErrMessagingDependency)
		return
	}
	image, err := h.service.UploadMessageImage(c.Request.Context(), application.MessageImageUploadRequest{UserID: c.GetUint("userID"), FileName: file.Filename, ContentType: file.Header.Get("Content-Type"), Size: file.Size, Reader: reader})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	messageSuccess(c, MessageImageUploadResponse{ID: image.ID, ExpiresAt: image.ExpiresAt})
}

func (h *handler) readImage(c *gin.Context) {
	messageID, ok := messagePathID(c)
	if !ok || h.service == nil {
		if h.service == nil {
			writeMessageError(c, application.ErrMessagingDependency)
		}
		return
	}
	imageID, err := strconv.ParseUint(c.Param("image_id"), 10, 64)
	if err != nil || imageID == 0 {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return
	}
	content, err := h.service.OpenVisibleMessageImage(c.Request.Context(), application.MessageImageReadRequest{UserID: c.GetUint("userID"), MessageID: messageID, ImageID: uint(imageID)})
	if err != nil {
		writeMessageError(c, err)
		return
	}
	if content.Reader == nil || (content.ContentType != "image/jpeg" && content.ContentType != "image/png" && content.ContentType != "image/webp") {
		if content.Reader != nil {
			_ = content.Reader.Close()
		}
		writeMessageError(c, application.ErrMessagingDependency)
		return
	}
	defer content.Reader.Close()
	c.Header("Content-Type", content.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	if content.Size > 0 {
		c.Header("Content-Length", strconv.FormatInt(content.Size, 10))
	}
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, content.Reader); err != nil {
		_ = c.Error(err)
	}
}

func parseInboxQuery(c *gin.Context, userID uint) (application.InboxQueryRequest, error) {
	page, err := positiveQueryInt(c, "page", 1)
	if err != nil {
		return application.InboxQueryRequest{}, err
	}
	limit, err := positiveQueryInt(c, "size", defaultMessagePageSize)
	if err != nil {
		return application.InboxQueryRequest{}, err
	}
	if limit > maxMessagePageSize {
		return application.InboxQueryRequest{}, application.ErrInboxRequestInvalid
	}
	read, err := optionalBoolQuery(c, "read")
	if err != nil {
		return application.InboxQueryRequest{}, err
	}
	categoryID, err := optionalUintQuery(c, "category_id")
	if err != nil {
		return application.InboxQueryRequest{}, err
	}
	kinds, err := messageKindsQuery(c)
	if err != nil {
		return application.InboxQueryRequest{}, err
	}
	return application.InboxQueryRequest{UserID: userID, Kinds: kinds, CategoryID: categoryID, Read: read, Keyword: c.Query("keyword"), Offset: (page - 1) * limit, Limit: limit}, nil
}

func messageKindsQuery(c *gin.Context) ([]domain.MessageKind, error) {
	values := c.QueryArray("kind")
	result := make([]domain.MessageKind, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			kind := domain.MessageKind(strings.TrimSpace(part))
			switch kind {
			case domain.MessageKindPrivate, domain.MessageKindBroadcast, domain.MessageKindAnnouncement:
				result = append(result, kind)
			case "":
			default:
				return nil, application.ErrInboxRequestInvalid
			}
		}
	}
	return result, nil
}

func positiveQueryInt(c *gin.Context, name string, defaultValue int) (int, error) {
	value := c.DefaultQuery(name, strconv.Itoa(defaultValue))
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, application.ErrInboxRequestInvalid
	}
	return parsed, nil
}

func optionalBoolQuery(c *gin.Context, name string) (*bool, error) {
	value := c.Query(name)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, application.ErrInboxRequestInvalid
	}
	return &parsed, nil
}

func optionalUintQuery(c *gin.Context, name string) (uint, error) {
	value := c.Query(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, application.ErrInboxRequestInvalid
	}
	return uint(parsed), nil
}

func messagePathID(c *gin.Context) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || parsed == 0 {
		writeMessageError(c, application.ErrInboxRequestInvalid)
		return 0, false
	}
	return uint(parsed), true
}

func parseMessageImage(c *gin.Context) (*multipart.FileHeader, func(), error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadsecurity.MaxMessageImageBytes+messageMultipartOverheadBytes)
	if err := c.Request.ParseMultipartForm(messageMultipartMemoryBytes); err != nil {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return nil, func() {}, application.ErrInboxRequestInvalid
		}
		return nil, func() {}, application.ErrInboxRequestInvalid
	}
	form := c.Request.MultipartForm
	cleanup := func() {
		if form != nil {
			_ = form.RemoveAll()
		}
	}
	files := form.File["file"]
	if len(files) != 1 {
		return nil, cleanup, application.ErrInboxRequestInvalid
	}
	if files[0].Size == 0 || files[0].Size > uploadsecurity.MaxMessageImageBytes {
		return nil, cleanup, application.ErrInboxRequestInvalid
	}
	return files[0], cleanup, nil
}
