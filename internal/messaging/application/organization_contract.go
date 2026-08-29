package application

import (
	"context"
	"time"
)

// OrganizationAudienceReader is the Messaging-owned view of current
// organization membership, descendant scope, and role audience membership.
// It exposes only caller-safe facts; Messaging resolves user eligibility
// through IdentityReader and never shares persistence models.
type OrganizationAudienceReader interface {
	Memberships(context.Context, uint) ([]OrganizationMembership, error)
	DescendantOrganizationIDs(context.Context, []uint) ([]uint, error)
	MemberUserIDs(context.Context, uint) ([]uint, error)
	RoleMemberUserIDs(context.Context, uint, uint) ([]uint, error)
}

// UserRoleIDsReader is an optional facet used by HTTP-facing application
// methods to replace caller-supplied role filters with current role
// membership. Implementations that do not expose it retain the legacy caller
// role filter for narrow unit-test seams.
type UserRoleIDsReader interface {
	RoleIDs(context.Context, uint) ([]uint, error)
}

// OrganizationMembership captures the current membership instant used to
// initialize an announcement as historical-read or newly-unread.
type OrganizationMembership struct {
	OrganizationID uint
	JoinedAt       time.Time
}
