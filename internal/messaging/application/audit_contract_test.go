package application_test

import (
	"context"
	"testing"

	"admin/internal/messaging/application"
)

type messagingAuditSinkFake struct {
	entries []application.MessagingAuditEntry
}

func (sink *messagingAuditSinkFake) Record(_ context.Context, entry application.MessagingAuditEntry) {
	sink.entries = append(sink.entries, entry)
}

var _ application.MessagingAuditSink = (*messagingAuditSinkFake)(nil)

func TestMessagingAuditContractCarriesOnlyControlledOperationMetadata(t *testing.T) {
	sink := &messagingAuditSinkFake{}
	sink.Record(context.Background(), application.MessagingAuditEntry{
		ActorID:        7,
		MessageCopyID:  8,
		OrganizationID: 9,
		Action:         "message.revoke",
		Result:         "denied",
		ReasonCode:     "MSG_SCOPE_DENIED",
	})
	if len(sink.entries) != 1 {
		t.Fatalf("recorded entries = %#v", sink.entries)
	}
	entry := sink.entries[0]
	if entry.ActorID != 7 || entry.MessageCopyID != 8 || entry.OrganizationID != 9 || entry.Action != "message.revoke" || entry.Result != "denied" || entry.ReasonCode != "MSG_SCOPE_DENIED" {
		t.Fatalf("audit entry = %#v", entry)
	}
}
