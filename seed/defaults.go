package seed

import "admin/model"

func defaultRoles() []seedRole {
	return []seedRole{
		{Name: "超级管理员", Code: "admin", Description: "系统最高权限角色", Sort: 0, Status: 1, DataScope: model.DataScopeAll},
		{Name: "普通用户", Code: "user", Description: "系统默认普通用户角色", Sort: 100, Status: 1, DataScope: model.DataScopeSelf},
	}
}

func defaultPermissionGroups() []seedPermissionGroup {
	return []seedPermissionGroup{
		{Name: "auth", Sort: 1},
		{Name: "user", Sort: 2},
		{Name: "role", Sort: 3},
		{Name: "permission", Sort: 4},
		{Name: "menu", Sort: 5},
		{Name: "api", Sort: 6},
		{Name: "organization", Sort: 7},
		{Name: "dict", Sort: 8},
		{Name: "file", Sort: 9},
		{Name: "audit", Sort: 10},
		{Name: "system", Sort: 99},
	}
}

func defaultMenus() []seedMenu {
	return []seedMenu{
		{Name: "系统管理", Path: "/system", Component: "Layout", Icon: "Settings", Sort: 10, Type: 1, Status: 1},
		{Name: "用户管理", Path: "/system/users", ParentPath: "/system", Component: "system/users/index", Icon: "Users", PermissionCode: "admin.users.get", Sort: 10, Type: 2, Status: 1},
		{Name: "角色管理", Path: "/system/roles", ParentPath: "/system", Component: "system/roles/index", Icon: "Shield", PermissionCode: "admin.roles.get", Sort: 20, Type: 2, Status: 1},
		{Name: "权限管理", Path: "/system/permissions", ParentPath: "/system", Component: "system/permissions/index", Icon: "KeyRound", PermissionCode: "admin.permissions.get", Sort: 30, Type: 2, Status: 1},
		{Name: "菜单管理", Path: "/system/menus", ParentPath: "/system", Component: "system/menus/index", Icon: "Menu", PermissionCode: "admin.menus.get", Sort: 40, Type: 2, Status: 1},
		{Name: "API 管理", Path: "/system/apis", ParentPath: "/system", Component: "system/apis/index", Icon: "Route", PermissionCode: "admin.apis.get", Sort: 50, Type: 2, Status: 1},
		{Name: "字典管理", Path: "/system/dicts", ParentPath: "/system", Component: "system/dicts/index", Icon: "BookOpen", PermissionCode: "admin.dict-types.get", Sort: 60, Type: 2, Status: 1},
		{Name: "组织管理", Path: "/system/organizations", ParentPath: "/system", Component: "system/organizations/index", Icon: "Network", PermissionCode: "admin.organizations.get", Sort: 70, Type: 2, Status: 1},
		{Name: "操作日志", Path: "/system/audit-logs", ParentPath: "/system", Component: "system/audit-logs/index", Icon: "FileClock", PermissionCode: "admin.audit-logs.get", Sort: 80, Type: 2, Status: 1},
		{Name: "文件管理", Path: "/system/files", ParentPath: "/system", Component: "system/files/index", Icon: "FolderOpen", PermissionCode: "admin.files.get", Sort: 90, Type: 2, Status: 1},
	}
}

func defaultDictTypes() []seedDictType {
	return []seedDictType{
		{
			Name: "用户状态", Code: "user_status", Remark: "用户启用状态", Sort: 10, Status: 1,
			Items: []seedDictItem{
				{Label: "禁用", Value: "0", Sort: 1, Status: 1},
				{Label: "启用", Value: "1", Sort: 2, Status: 1},
			},
		},
		{
			Name: "角色状态", Code: "role_status", Remark: "角色启用状态", Sort: 20, Status: 1,
			Items: []seedDictItem{
				{Label: "禁用", Value: "0", Sort: 1, Status: 1},
				{Label: "启用", Value: "1", Sort: 2, Status: 1},
			},
		},
		{
			Name: "菜单类型", Code: "menu_type", Remark: "后台菜单节点类型", Sort: 30, Status: 1,
			Items: []seedDictItem{
				{Label: "目录", Value: "1", Sort: 1, Status: 1},
				{Label: "菜单", Value: "2", Sort: 2, Status: 1},
				{Label: "按钮", Value: "3", Sort: 3, Status: 1},
			},
		},
		{
			Name: "HTTP 方法", Code: "api_method", Remark: "API 请求方法", Sort: 40, Status: 1,
			Items: []seedDictItem{
				{Label: "GET", Value: "GET", Sort: 1, Status: 1},
				{Label: "POST", Value: "POST", Sort: 2, Status: 1},
				{Label: "PUT", Value: "PUT", Sort: 3, Status: 1},
				{Label: "PATCH", Value: "PATCH", Sort: 4, Status: 1},
				{Label: "DELETE", Value: "DELETE", Sort: 5, Status: 1},
			},
		},
		{
			Name: "数据范围", Code: "data_scope", Remark: "角色数据权限范围", Sort: 50, Status: 1,
			Items: []seedDictItem{
				{Label: "全部数据", Value: model.DataScopeAll, Sort: 1, Status: 1},
				{Label: "仅本人数据", Value: model.DataScopeSelf, Sort: 2, Status: 1},
				{Label: "本组织数据", Value: model.DataScopeOrg, Sort: 3, Status: 1},
				{Label: "本组织及下级组织", Value: model.DataScopeOrgAndChildren, Sort: 4, Status: 1},
				{Label: "自定义组织", Value: model.DataScopeCustom, Sort: 5, Status: 1},
			},
		},
	}
}
