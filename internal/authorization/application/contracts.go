package application

import (
	"context"

	"admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
)

// TransactionRunner is owned by the caller's application boundary. Implementations
// join an existing transaction carried by ctx and own only database work.
type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}

// OrganizationScopeReader is the smallest organization capability required by
// authorization data-scope evaluation. It intentionally exposes no persistence type.
type OrganizationScopeReader interface {
	MemberOrganizationIDs(context.Context, uint) ([]uint, error)
	DescendantOrganizationIDs(context.Context, []uint) ([]uint, error)
	ExistingOrganizationIDs(context.Context, []uint) ([]uint, error)
	MemberUserIDs(context.Context, uint) ([]uint, error)
}

// UserDirectory is the identity capability used for password-free role-user responses.
type UserDirectory interface {
	ListUsersByIDs(context.Context, []uint) ([]identitydomain.DirectoryUser, error)
}

// AccessVersionStore is the authorization-owned invalidation capability consumed
// by authorization and identity use cases.
type AccessVersionStore interface {
	Current(context.Context, uint) (int, error)
	Ensure(context.Context, uint) (int, error)
	EnsureAndIncrement(context.Context, uint) (int, error)
}

// AuthorizationReader is the capability consumed by identity and middleware.
type AuthorizationReader interface {
	Snapshot(context.Context, domain.Principal) (domain.AccessSnapshot, error)
	Permissions(context.Context, uint) ([]string, error)
}

// RoutePermissionSource supplies static route facts without exposing Gin state.
type RoutePermissionSource interface {
	Routes(context.Context) ([]domain.RouteFact, error)
}
