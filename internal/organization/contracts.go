package organization

import "context"

type OrganizationScope struct {
	All             bool
	OrganizationIDs []uint
}

type VisibilityProvider interface {
	Scope(context.Context, uint) (OrganizationScope, error)
}

type UserDirectory interface {
	ExistingUserIDs(context.Context, []uint) ([]uint, error)
	ListUsersByIDs(context.Context, []uint) ([]MemberInfo, error)
}

type AccessVersionInvalidator interface {
	Increment(context.Context, []uint) error
}

type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}
