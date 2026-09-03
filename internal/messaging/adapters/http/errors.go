package httpadapter

import (
	"errors"
	"net/http"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

const (
	CodeRequestInvalid       = "MSG_REQUEST_INVALID"
	CodeRecipientUnavailable = "MSG_RECIPIENT_UNAVAILABLE"
	CodePermissionDenied     = "MSG_PERMISSION_DENIED"
	CodeScopeDenied          = "MSG_SCOPE_DENIED"
	CodeNotFound             = "MSG_NOT_FOUND"
	CodeStateConflict        = "MSG_STATE_CONFLICT"
	CodeCategoryUnavailable  = "MSG_CATEGORY_UNAVAILABLE"
	CodeCategoryInUse        = "MSG_CATEGORY_IN_USE"
	CodeAudienceInvalid      = "MSG_AUDIENCE_INVALID"
	CodeAudienceTooLarge     = "MSG_AUDIENCE_TOO_LARGE"
	CodeContentEmpty         = "MSG_CONTENT_EMPTY"
	CodeContentUnsafe        = "MSG_CONTENT_UNSAFE"
	CodeTitleTooLong         = "MSG_TITLE_TOO_LONG"
	CodeBodyTooLong          = "MSG_BODY_TOO_LONG"
	CodeHTMLTooLarge         = "MSG_HTML_TOO_LARGE"
	CodeTextInvalid          = "MSG_TEXT_INVALID"
	CodeMessageInvalid       = "MSG_MESSAGE_INVALID"
	CodeMediaInvalid         = "MSG_MEDIA_INVALID"
	CodeTicketInvalid        = "MSG_WS_TICKET_INVALID"
	CodeWebSocketUnavailable = "MSG_WEBSOCKET_UNAVAILABLE"
	CodeInternalError        = "MSG_INTERNAL_ERROR"
)

func MessageErrorDefinitions() []httpresponse.ErrorDefinition {
	return []httpresponse.ErrorDefinition{
		messageError(CodeRequestInvalid, http.StatusBadRequest, "message request is invalid"),
		messageError(CodeRecipientUnavailable, http.StatusUnprocessableEntity, "message recipient is unavailable"),
		messageError(CodePermissionDenied, http.StatusForbidden, "message permission is denied"),
		messageError(CodeScopeDenied, http.StatusForbidden, "message organization is outside the allowed scope"),
		messageError(CodeNotFound, http.StatusNotFound, "message resource was not found"),
		messageError(CodeStateConflict, http.StatusConflict, "message state conflicts with the requested operation"),
		messageError(CodeCategoryUnavailable, http.StatusUnprocessableEntity, "message category is unavailable"),
		messageError(CodeCategoryInUse, http.StatusConflict, "message category is in use"),
		messageError(CodeAudienceInvalid, http.StatusUnprocessableEntity, "message audience is invalid"),
		messageError(CodeAudienceTooLarge, http.StatusUnprocessableEntity, "message audience is too large"),
		messageError(CodeContentEmpty, http.StatusUnprocessableEntity, "message content is empty"),
		messageError(CodeContentUnsafe, http.StatusUnprocessableEntity, "message content is unsafe"),
		messageError(CodeTitleTooLong, http.StatusUnprocessableEntity, "message title is too long"),
		messageError(CodeBodyTooLong, http.StatusUnprocessableEntity, "message body is too long"),
		messageError(CodeHTMLTooLarge, http.StatusUnprocessableEntity, "message HTML is too large"),
		messageError(CodeTextInvalid, http.StatusUnprocessableEntity, "message text is invalid"),
		messageError(CodeMessageInvalid, http.StatusUnprocessableEntity, "message is invalid"),
		messageError(CodeMediaInvalid, http.StatusUnprocessableEntity, "message media is invalid"),
		messageError(CodeTicketInvalid, http.StatusUnauthorized, "message WebSocket ticket is invalid"),
		messageError(CodeWebSocketUnavailable, http.StatusServiceUnavailable, "message WebSocket is unavailable"),
		messageError(CodeInternalError, http.StatusInternalServerError, "message operation failed"),
	}
}

func messageError(code string, status int, message string) httpresponse.ErrorDefinition {
	return httpresponse.ErrorDefinition{Owner: "messaging", Code: code, Status: status, Message: message}
}

func classifyMessageError(err error) httpresponse.ErrorDefinition {
	switch {
	case err == nil:
		return messageError(CodeInternalError, http.StatusInternalServerError, "message operation failed")
	case errors.Is(err, application.ErrInboxRequestInvalid):
		return messageError(CodeRequestInvalid, http.StatusBadRequest, "message request is invalid")
	case errors.Is(err, application.ErrRecipientUnavailable):
		return messageError(CodeRecipientUnavailable, http.StatusUnprocessableEntity, "message recipient is unavailable")
	case errors.Is(err, application.ErrPermissionDenied), errors.Is(err, application.ErrRevokeNotAllowed):
		return messageError(CodePermissionDenied, http.StatusForbidden, "message permission is denied")
	case errors.Is(err, application.ErrOrganizationNotManaged):
		return messageError(CodeScopeDenied, http.StatusForbidden, "message organization is outside the allowed scope")
	case errors.Is(err, application.ErrNotFound):
		return messageError(CodeNotFound, http.StatusNotFound, "message resource was not found")
	case errors.Is(err, application.ErrStateConflict), errors.Is(err, application.ErrMessageImmutable), errors.Is(err, application.ErrLeaseNotHeld), errors.Is(err, application.ErrConsumerDeadLetterInvalid):
		return messageError(CodeStateConflict, http.StatusConflict, "message state conflicts with the requested operation")
	case errors.Is(err, application.ErrCategoryInUse):
		return messageError(CodeCategoryInUse, http.StatusConflict, "message category is in use")
	case errors.Is(err, application.ErrCategoryUnavailable):
		return messageError(CodeCategoryUnavailable, http.StatusUnprocessableEntity, "message category is unavailable")
	case errors.Is(err, application.ErrBroadcastAudienceInvalid), errors.Is(err, domain.ErrAudienceRuleInvalid):
		return messageError(CodeAudienceInvalid, http.StatusUnprocessableEntity, "message audience is invalid")
	case errors.Is(err, application.ErrAudienceBatchTooLarge), errors.Is(err, domain.ErrAudienceCapacityExceeded):
		return messageError(CodeAudienceTooLarge, http.StatusUnprocessableEntity, "message audience is too large")
	case errors.Is(err, domain.ErrMessageContentEmpty):
		return messageError(CodeContentEmpty, http.StatusUnprocessableEntity, "message content is empty")
	case errors.Is(err, domain.ErrMessageContentUnsafe):
		return messageError(CodeContentUnsafe, http.StatusUnprocessableEntity, "message content is unsafe")
	case errors.Is(err, domain.ErrMessageTitleTooLong):
		return messageError(CodeTitleTooLong, http.StatusUnprocessableEntity, "message title is too long")
	case errors.Is(err, domain.ErrMessageBodyTooLong):
		return messageError(CodeBodyTooLong, http.StatusUnprocessableEntity, "message body is too long")
	case errors.Is(err, domain.ErrMessageHTMLTooLarge):
		return messageError(CodeHTMLTooLarge, http.StatusUnprocessableEntity, "message HTML is too large")
	case errors.Is(err, domain.ErrMessageTextInvalid):
		return messageError(CodeTextInvalid, http.StatusUnprocessableEntity, "message text is invalid")
	case errors.Is(err, domain.ErrMessageIdentityInvalid), errors.Is(err, domain.ErrMessageStatusInvalid), errors.Is(err, domain.ErrMessageDatesInvalid):
		return messageError(CodeMessageInvalid, http.StatusUnprocessableEntity, "message is invalid")
	case errors.Is(err, domain.ErrPrivateRecipientInvalid):
		return messageError(CodeRecipientUnavailable, http.StatusUnprocessableEntity, "message recipient is unavailable")
	case errors.Is(err, application.ErrWebSocketTicketInvalid):
		return messageError(CodeTicketInvalid, http.StatusUnauthorized, "message WebSocket ticket is invalid")
	case errors.Is(err, application.ErrWebSocketGatewayUnavailable), errors.Is(err, application.ErrWebSocketQueueFull):
		return messageError(CodeWebSocketUnavailable, http.StatusServiceUnavailable, "message WebSocket is unavailable")
	case errors.Is(err, application.ErrMessagingDependency):
		return messageError(CodeInternalError, http.StatusInternalServerError, "message operation failed")
	default:
		return messageError(CodeInternalError, http.StatusInternalServerError, "message operation failed")
	}
}

func writeMessageError(c *gin.Context, err error) {
	httpresponse.WriteError(c, classifyMessageError(err), err, nil)
}

func messageSuccess(c *gin.Context, data any) {
	httpresponse.WriteSuccess(c, http.StatusOK, data)
}
