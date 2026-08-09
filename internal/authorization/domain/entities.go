package domain

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
