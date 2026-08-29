package application

import "context"

// AuthorizationReader is the Messaging-owned authorization boundary. It
// exposes permission decisions and a resource scope, not roles, HTTP state, or
// authorization persistence models.
type AuthorizationReader interface {
	HasPermission(context.Context, uint, string) (bool, error)
	OrganizationScope(context.Context, uint) (MessageOrganizationScope, error)
}

// MessageOrganizationScope is the set of organization units an operator may
// manage. All is reserved for the existing super-administrator authorization
// decision.
type MessageOrganizationScope struct {
	All             bool
	OrganizationIDs []uint
}
