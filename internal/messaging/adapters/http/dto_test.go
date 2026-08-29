package httpadapter

import (
	"reflect"
	"testing"
	"time"

	"admin/internal/messaging/application"
	"admin/internal/messaging/domain"
)

func TestMessageDTOMapsRenderedContentAndReadStateWithoutMarkdown(t *testing.T) {
	readAt := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	message := application.InboxMessage{Message: domain.Message{ID: 41, OrganizationID: 10, SenderID: 7, CategoryID: 3, Kind: domain.MessageKindAnnouncement, Status: domain.MessageStatusPublished, Title: "Notice", BodyHTML: "<p>Safe</p>", PublishAt: &readAt, AggregateVersion: 2}, ReadAt: &readAt}
	dto := toMessageDTO(message)
	if dto.ID != 41 || dto.OrganizationID != 10 || dto.SenderID != 7 || dto.CategoryID != 3 || dto.Kind != string(domain.MessageKindAnnouncement) || dto.Status != string(domain.MessageStatusPublished) || dto.Title != "Notice" || dto.BodyHTML != "<p>Safe</p>" || dto.ReadAt == nil || dto.AggregateVersion != 2 {
		t.Fatalf("message DTO = %#v", dto)
	}
	if dto.RevokedAt != nil || dto.ExpiresAt != nil {
		t.Fatalf("message DTO status fields = %#v", dto)
	}
}

func TestMessageDTORedactsBodyForRevokedOrExpiredMessages(t *testing.T) {
	for _, status := range []domain.MessageStatus{domain.MessageStatusRevoked, domain.MessageStatusExpired} {
		dto := toMessageDTO(application.InboxMessage{Message: domain.Message{Status: status, BodyHTML: "<p>private body</p>"}})
		if dto.Status != string(status) || dto.BodyHTML != "" {
			t.Fatalf("status %q DTO = %#v; body must be redacted", status, dto)
		}
	}
}

func TestMessageRequestDTOsUseStableJSONFieldNames(t *testing.T) {
	request := SendPrivateMessageRequest{RecipientID: 8, Title: "hello", Markdown: "body"}
	if request.RecipientID != 8 || request.Title != "hello" || request.Markdown != "body" {
		t.Fatalf("private request = %#v", request)
	}
	if _, ok := reflect.TypeOf(MessageDTO{}).FieldByName("Markdown"); ok {
		t.Fatal("MessageDTO must not expose Markdown")
	}
}
