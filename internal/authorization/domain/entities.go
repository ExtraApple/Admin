package domain

import "time"

type Role struct {
	ID          uint
	Name        string
	Code        string
	Description string
	Sort        int
	Status      int
	DataScope   DataScope
}

type Permission struct {
	ID    uint
	Name  string
	Code  PermissionCode
	Group string
	Sort  int
}

type PermissionGroup struct {
	ID   uint
	Name string
	Sort int
}

type UserSummary struct {
	ID          uint
	Username    string
	Nickname    string
	Avatar      string
	Email       string
	Role        string
	Status      int
	LastLoginAt *time.Time
	CreatedAt   time.Time
}

type RoleDataScope struct {
	RoleID          uint
	DataScope       DataScope
	OrganizationIDs []uint
}

type RouteFact struct {
	Method         string
	Path           string
	PermissionCode string
	Public         bool
}
