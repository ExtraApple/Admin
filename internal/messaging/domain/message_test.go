package domain_test

import (
	"testing"
	"time"

	"admin/internal/messaging/domain"
)

func TestMessageCopyValidationKeepsOnlyCompiledMessageContent(t *testing.T) {
	message := domain.Message{
		LogicalID:      "logical-1",
		OrganizationID: 10,
		SenderID:       20,
		CategoryID:     30,
		Kind:           domain.MessageKindAnnouncement,
		Status:         domain.MessageStatusDraft,
		Title:          "公告标题",
		BodyHTML:       "<p>已清洗正文</p>",
	}
	if err := domain.ValidateMessage(message); err != nil {
		t.Fatalf("ValidateMessage() error = %v", err)
	}
	if message.BodyHTML == "" {
		t.Fatal("message body HTML must be retained as the persisted content")
	}
}

func TestMessageCopyValidationRejectsInvalidLifecycleAndDates(t *testing.T) {
	publishAt := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	expiresAt := publishAt.Add(-time.Hour)
	message := domain.Message{
		LogicalID:      "logical-1",
		OrganizationID: 10,
		SenderID:       20,
		Kind:           domain.MessageKindAnnouncement,
		Status:         domain.MessageStatusPublished,
		Title:          "公告标题",
		BodyHTML:       "<p>正文</p>",
		PublishAt:      &publishAt,
		ExpiresAt:      &expiresAt,
	}
	if err := domain.ValidateMessage(message); err != domain.ErrMessageDatesInvalid {
		t.Fatalf("ValidateMessage() error = %v, want ErrMessageDatesInvalid", err)
	}

	message.ExpiresAt = nil
	message.Status = domain.MessageStatusDraft
	message.Kind = domain.MessageKindPrivate
	if err := domain.ValidateMessage(message); err != domain.ErrMessageStatusInvalid {
		t.Fatalf("ValidateMessage() error = %v, want ErrMessageStatusInvalid", err)
	}
}

func TestAudienceRuleValidationSeparatesOrganizationAndRoleAudience(t *testing.T) {
	for _, rule := range []domain.AudienceRule{
		{Type: domain.AudienceTypeOrganization, OrganizationID: 10},
		{Type: domain.AudienceTypeRole, OrganizationID: 10, RoleID: 20},
		{Type: domain.AudienceTypeAll, OrganizationID: 10},
	} {
		if err := domain.ValidateAudienceRule(rule); err != nil {
			t.Fatalf("ValidateAudienceRule(%+v) error = %v", rule, err)
		}
	}

	invalid := domain.AudienceRule{Type: domain.AudienceTypeRole, OrganizationID: 10}
	if err := domain.ValidateAudienceRule(invalid); err != domain.ErrAudienceRuleInvalid {
		t.Fatalf("ValidateAudienceRule() error = %v, want ErrAudienceRuleInvalid", err)
	}
}

func TestPrivateRecipientValidationRejectsSelfRecipient(t *testing.T) {
	recipient := domain.PrivateRecipient{MessageID: 1, SenderID: 20, RecipientID: 20}
	if err := domain.ValidatePrivateRecipient(recipient); err != domain.ErrPrivateRecipientInvalid {
		t.Fatalf("ValidatePrivateRecipient() error = %v, want ErrPrivateRecipientInvalid", err)
	}
}
