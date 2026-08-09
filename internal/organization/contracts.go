package organization

import (
	"context"

	identitydomain "admin/internal/identity/domain"
)

type OrganizationScope struct {
	All             bool
	OrganizationIDs []uint
}

type VisibilityProvider interface {
	Scope(context.Context, uint) (OrganizationScope, error)
}

type UserDirectory interface {
	ListUsersByIDs(context.Context, []uint) ([]identitydomain.DirectoryUser, error)
}

type AccessVersionInvalidator interface {
	Increment(context.Context, []uint) error
}

type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}
