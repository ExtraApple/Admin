package model

// Models 模型注册表 — 新增模型只需在这里追加
var Models = []interface{}{
	User{},
	Role{},
	UserRole{},
	Permission{},
	RolePermission{},
	PermissionGroup{},
	File{},
	Menu{},
	RoleMenu{},
	AuditLog{},
	AuditLogArchive{},
	DictType{},
	DictItem{},
	Organization{},
	UserOrganization{},
}
