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
	CurrentScope(context.Context, uint) (OrganizationScope, error)
}

type UserDirectory interface {
	ListUsersByIDs(context.Context, []uint) ([]identitydomain.DirectoryUser, error)
}

type AccessVersionInvalidator interface {
	Increment(context.Context, []uint) error
}

// OrganizationReferencePolicy joins the caller's hierarchy-locked transaction.
// It locks current operator/member and role dependencies before scope checks.
type OrganizationReferencePolicy interface {
	LockOrganizationReferences(context.Context, uint, uint, []uint) (bool, error)
}

// HierarchyDependencyReader joins the caller's hierarchy-locked transaction and
// returns current users whose role/member roots depend on the changed hierarchy.
type HierarchyDependencyReader interface {
	HierarchyDependentUserIDs(context.Context, []uint) ([]uint, error)
}

type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}
