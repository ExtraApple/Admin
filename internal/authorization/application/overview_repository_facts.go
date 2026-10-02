package application

import "context"

type RolePermissionFact struct {
	RoleID uint
	Code   string
}

type OverviewUserDirectory interface {
	UserDirectory
	ListUserIDs(context.Context) ([]uint, error)
}
