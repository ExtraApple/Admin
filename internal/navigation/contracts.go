package navigation

import "context"

type TransactionRunner interface {
	Run(context.Context, func(context.Context) error) error
}

type Authorization interface {
	RoleExists(context.Context, uint) (bool, error)
	UserAccess(context.Context, uint) (UserAccess, error)
	UserIDsByRoleIDs(context.Context, []uint) ([]uint, error)
	AllUserIDs(context.Context) ([]uint, error)
	RoleIDsByPermissionCode(context.Context, string) ([]uint, error)
	LockPermission(context.Context, string) (PermissionRef, bool, error)
	EnsurePermission(context.Context, PermissionSeed) (PermissionRef, bool, error)
	MergePermissionRoles(context.Context, uint, uint) error
	PermissionHasRoleReferences(context.Context, uint) (bool, error)
	DeletePermission(context.Context, uint) error
	IncrementAccessVersions(context.Context, []uint) error
}

type APIMetadataReader interface {
	ListByIDs(context.Context, []uint) ([]APIRecord, error)
	Lock(context.Context, uint) (APIRecord, error)
	LockMany(context.Context, []uint) ([]APIRecord, error)
	SetPermissionCode(context.Context, []uint, string) error
	CountPermissionCode(context.Context, string) (int64, error)
}
type CacheInvalidator interface {
	Invalidate(context.Context, []uint) error
}

type Metrics interface {
	RecordCacheInvalidationFailure()
}

type ServiceOption func(*Service)

func WithCacheInvalidation(cache CacheInvalidator, metrics Metrics) ServiceOption {
	return func(service *Service) {
		service.cache = cache
		service.metrics = metrics
	}
}
