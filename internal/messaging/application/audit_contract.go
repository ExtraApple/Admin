package application

import "context"

// MessagingAuditSink records the allowlisted metadata of a Messaging operation.
// Implementations are supplied by App and must not make business completion
// depend on Audit availability.
type MessagingAuditSink interface {
	Record(context.Context, MessagingAuditEntry)
}

// MessagingAuditEntry excludes message content, external URLs, media bytes,
// credentials, and infrastructure errors.
type MessagingAuditEntry struct {
	ActorID        uint
	MessageCopyID  uint
	OrganizationID uint
	EventID        string
	ConsumerName   string
	ReplayCycle    uint
	Action         string
	Result         string
	ReasonCode     string
}
