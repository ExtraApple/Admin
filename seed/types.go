package seed

type seedRole struct {
	Name        string
	Code        string
	Description string
	Sort        int
	Status      int
	DataScope   string
}

type seedPermissionGroup struct {
	Name string
	Sort int
}

type seedDictType struct {
	Name   string
	Code   string
	Remark string
	Sort   int
	Status int
	Items  []seedDictItem
}

type seedDictItem struct {
	Label  string
	Value  string
	Remark string
	Sort   int
	Status int
}

type seedMenu struct {
	Name           string
	Path           string
	ParentPath     string
	Component      string
	Icon           string
	PermissionCode string
	Sort           int
	Type           int
	Status         int
}

type seedSummary struct {
	Roles            int
	PermissionGroups int
	APIs             int
	Permissions      int
	Menus            int
	RolePermissions  int
	RoleMenus        int
	DictTypes        int
	DictItems        int
}
