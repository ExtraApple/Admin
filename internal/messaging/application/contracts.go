package application

import (
	"context"
	"io"
	"time"
)

// IdentityReader is the Messaging-owned view of Identity capabilities. It
// excludes persistence, credentials, and email-verification internals.
type IdentityReader interface {
	LookupUser(context.Context, uint) (IdentityUser, error)
	LookupVerifiedEmail(context.Context, uint) (VerifiedEmail, bool, error)
}

// IdentityUser is the minimum identity fact required to address or validate a
// Messaging participant.
type IdentityUser struct {
	ID          uint
	DisplayName string
	Enabled     bool
}

// VerifiedEmail is the only email address a notification adapter may receive.
type VerifiedEmail struct {
	Address string
}

// OrganizationAudienceReader is the Messaging-owned view of current
// organization membership, descendant scope, and role audience membership.
type OrganizationAudienceReader interface {
	Memberships(context.Context, uint) ([]OrganizationMembership, error)
	DescendantOrganizationIDs(context.Context, []uint) ([]uint, error)
	MemberUserIDs(context.Context, uint) ([]uint, error)
	RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error)
}

// UserRoleIDsReader is an optional facet used to resolve current role filters.
type UserRoleIDsReader interface {
	RoleIDs(context.Context, uint) ([]uint, error)
}

// OrganizationMembership captures the current membership instant used to
// initialize first-visible announcement state.
type OrganizationMembership struct {
	OrganizationID uint
	JoinedAt       time.Time
}

// AuthorizationReader is the Messaging-owned authorization boundary. It
// exposes permission decisions and resource scope, not persistence models.
type AuthorizationReader interface {
	HasPermission(context.Context, uint, string) (bool, error)
	OrganizationScope(context.Context, uint) (MessageOrganizationScope, error)
}

// MessageOrganizationScope is the organization set an operator may manage.
type MessageOrganizationScope struct {
	All             bool
	OrganizationIDs []uint
}

// MessageImageFiles is the Messaging-owned view of Files. It provides only
// temporary upload, atomic binding, and an already-authorized content read.
type MessageImageFiles interface {
	UploadMessageImage(context.Context, MessageImageUpload) (TemporaryMessageImage, error)
	BindMessageImages(context.Context, MessageImageBinding) error
	OpenMessageImage(context.Context, VisibleMessageImage) (MessageImageContent, error)
}

// MessageImageUpload is an unbound image owned by its uploader until binding.
type MessageImageUpload struct {
	UploaderID            uint
	FileName, ContentType string
	Size                  int64
	Reader                io.Reader
}

// TemporaryMessageImage is a validated unbound File Record.
type TemporaryMessageImage struct {
	ID        uint
	ExpiresAt time.Time
}

// MessageImageBinding atomically gives selected temporary images one logical
// message owner. Files accepts only unexpired images owned by ActorID.
type MessageImageBinding struct {
	ActorID          uint
	MessageLogicalID string
	ImageIDs         []uint
}

// VisibleMessageImage is constructed after Messaging determines visibility.
type VisibleMessageImage struct {
	ID               uint
	MessageLogicalID string
}

// MessageImageContent contains a validated image stream and canonical MIME.
type MessageImageContent struct {
	Reader      io.ReadCloser
	ContentType string
	Size        int64
}

// MessagingMetrics is a best-effort operational observation seam. Its method
// never returns an error and never influences persistence or acknowledgement.
type MessagingMetrics interface {
	RecordConsumerDLQPending(context.Context, ConsumerDLQPendingObservation)
}

// ConsumerDLQPendingObservation contains only safe failure dimensions.
type ConsumerDLQPendingObservation struct {
	ConsumerName     string
	FailureCode      string
	PendingCount     int64
	OldestPendingAge time.Duration
}
