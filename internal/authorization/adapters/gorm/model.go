package gormadapter

// Models returns the persistence models owned by Authorization.
func Models() []any {
	return []any{
		Role{},
		UserRole{},
		RoleDataScope{},
		Permission{},
		RolePermission{},
		PermissionGroup{},
		UserAccessVersion{},
	}
}
