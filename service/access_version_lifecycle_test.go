package service

import (
	"testing"

	"admin/dto"
	"admin/model"
)

func TestRegisteringUserDoesNotPrecreateAccessVersion(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	redisClient := installTokenVersionRegressionGlobals(t, db)
	const (
		captchaID   = "register-without-access-version"
		captchaCode = "123456"
	)
	redisClient.AddHook(tokenVersionRegressionRedisHook{
		captchaID:   captchaID,
		captchaCode: captchaCode,
	})

	registered, err := Register(dto.RegisterReq{
		Username:    "lazy-access-version-user",
		Password:    "ValidPass123!",
		Email:       "lazy-access-version-user@example.com",
		Nickname:    "Lazy Version",
		CaptchaID:   captchaID,
		CaptchaCode: captchaCode,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registered == nil || registered.ID == 0 {
		t.Fatalf("Register() response = %#v, want persisted user", registered)
	}

	var accessVersionCount int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", registered.ID).
		Count(&accessVersionCount).Error; err != nil {
		t.Fatalf("count access versions after registration: %v", err)
	}
	if accessVersionCount != 0 {
		t.Fatalf(
			"access-version rows after registration = %d, want lazy 0",
			accessVersionCount,
		)
	}
}

func TestRoleChangeUsesAuthorizationVersion(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	role := model.Role{
		Name:      "role-change",
		Code:      "role-change",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "role-change-user",
		Password:     "not-used",
		Email:        "role-change-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 4,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create role user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: user.ID,
		RoleID: role.ID,
	}).Error; err != nil {
		t.Fatalf("assign role user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create role user access version: %v", err)
	}

	if _, err := UpdateRole(role.ID, dto.UpdateRoleReq{
		Description: "updated description",
	}); err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestUserRoleChangeUsesAuthorizationVersionForRemovedAndAddedUsers(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	role := model.Role{
		Name:      "membership-role",
		Code:      "membership-role",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create membership role: %v", err)
	}
	oldUser := exitAccessVersionTestUser{
		Username:     "old-role-user",
		Password:     "not-used",
		Email:        "old-role-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 2,
	}
	newUser := exitAccessVersionTestUser{
		Username:     "new-role-user",
		Password:     "not-used",
		Email:        "new-role-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 5,
	}
	if err := db.Create(&oldUser).Error; err != nil {
		t.Fatalf("create removed role user: %v", err)
	}
	if err := db.Create(&newUser).Error; err != nil {
		t.Fatalf("create added role user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: oldUser.ID,
		RoleID: role.ID,
	}).Error; err != nil {
		t.Fatalf("assign old role user: %v", err)
	}
	for _, user := range []exitAccessVersionTestUser{oldUser, newUser} {
		if err := db.Create(&model.UserAccessVersion{
			UserID:  user.ID,
			Version: user.TokenVersion,
		}).Error; err != nil {
			t.Fatalf("create %s access version: %v", user.Username, err)
		}
	}

	if err := AssignUsersToRole(role.ID, []uint{newUser.ID}); err != nil {
		t.Fatalf("AssignUsersToRole() error = %v", err)
	}

	for _, user := range []exitAccessVersionTestUser{oldUser, newUser} {
		assertTokenVersionRegressionState(
			t,
			db,
			user.ID,
			1,
			user.TokenVersion,
		)
		assertAuthorizationVersion(
			t,
			db,
			user.ID,
			user.TokenVersion+1,
		)
	}
}

func TestRoleMenuChangeUsesAuthorizationVersion(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	role := model.Role{
		Name:      "menu-role",
		Code:      "menu-role",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create menu role: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "menu-role-user",
		Password:     "not-used",
		Email:        "menu-role-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 6,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create menu role user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: user.ID,
		RoleID: role.ID,
	}).Error; err != nil {
		t.Fatalf("assign menu role user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create menu role user access version: %v", err)
	}
	path := "/menu-role"
	menu := model.Menu{
		Name:   "menu role entry",
		Path:   &path,
		Type:   2,
		Status: 1,
	}
	if err := db.Create(&menu).Error; err != nil {
		t.Fatalf("create menu role entry: %v", err)
	}

	if err := AssignMenusToRole(role.ID, []uint{menu.ID}); err != nil {
		t.Fatalf("AssignMenusToRole() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestDeletingPermissionUsesAuthorizationVersionForAffectedRoleUsers(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	role := model.Role{
		Name:      "permission-delete-role",
		Code:      "permission-delete-role",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create permission role: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "permission-delete-user",
		Password:     "not-used",
		Email:        "permission-delete-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 3,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create permission user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: user.ID,
		RoleID: role.ID,
	}).Error; err != nil {
		t.Fatalf("assign permission role: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create permission user access version: %v", err)
	}
	permission := model.Permission{
		Name: "permission to delete",
		Code: "permission.delete",
	}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := db.Create(&model.RolePermission{
		RoleID:       role.ID,
		PermissionID: permission.ID,
	}).Error; err != nil {
		t.Fatalf("assign permission: %v", err)
	}

	if err := DeletePermission(permission.ID); err != nil {
		t.Fatalf("DeletePermission() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestDeletingRoleUsesAuthorizationVersionForFormerUsers(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	role := model.Role{
		Name:      "role-to-delete",
		Code:      "role-to-delete",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "former-role-user",
		Password:     "not-used",
		Email:        "former-role-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 8,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create role user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: user.ID,
		RoleID: role.ID,
	}).Error; err != nil {
		t.Fatalf("assign role user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create role user access version: %v", err)
	}

	if err := DeleteRole(role.ID); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestRoleDataScopeChangeUsesAuthorizationVersion(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	role := model.Role{
		Name:      "data-scope-role",
		Code:      "data-scope-role",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create data-scope role: %v", err)
	}
	user := exitAccessVersionTestUser{
		Username:     "data-scope-user",
		Password:     "not-used",
		Email:        "data-scope-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 5,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create data-scope user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: user.ID,
		RoleID: role.ID,
	}).Error; err != nil {
		t.Fatalf("assign data-scope role: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create data-scope user access version: %v", err)
	}

	if err := AssignRoleDataScope(role.ID, dto.AssignRoleDataScopeReq{
		DataScope: model.DataScopeOrg,
	}); err != nil {
		t.Fatalf("AssignRoleDataScope() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestCreatingMenuUsesAuthorizationVersionForAllUsers(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "menu-create-user",
		Password:     "not-used",
		Email:        "menu-create-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 2,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create menu user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create menu user access version: %v", err)
	}

	if _, err := CreateMenu(dto.CreateMenuReq{
		Name: "new menu",
		Path: "/new-menu",
		Type: 2,
	}); err != nil {
		t.Fatalf("CreateMenu() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestUpdatingMenuUsesAuthorizationVersionForAllUsers(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "menu-update-user",
		Password:     "not-used",
		Email:        "menu-update-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 4,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create menu user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create menu user access version: %v", err)
	}
	path := "/menu-before-update"
	menu := model.Menu{
		Name:   "menu before update",
		Path:   &path,
		Type:   2,
		Status: 1,
	}
	if err := db.Create(&menu).Error; err != nil {
		t.Fatalf("create menu: %v", err)
	}

	if _, err := UpdateMenu(menu.ID, dto.UpdateMenuReq{
		Name: "menu after update",
	}); err != nil {
		t.Fatalf("UpdateMenu() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestDeletingMenuUsesAuthorizationVersionForAllUsers(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "menu-delete-user",
		Password:     "not-used",
		Email:        "menu-delete-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 6,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create menu user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create menu user access version: %v", err)
	}
	path := "/menu-to-delete"
	menu := model.Menu{
		Name:   "menu to delete",
		Path:   &path,
		Type:   2,
		Status: 1,
	}
	if err := db.Create(&menu).Error; err != nil {
		t.Fatalf("create menu: %v", err)
	}

	if err := DeleteMenu(menu.ID); err != nil {
		t.Fatalf("DeleteMenu() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestSyncingNewMenusUsesAuthorizationVersionForAllUsers(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "menu-sync-user",
		Password:     "not-used",
		Email:        "menu-sync-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 7,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create menu user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create menu user access version: %v", err)
	}

	created, err := SyncMenus([]dto.SyncMenuItem{{
		Name: "synced menu",
		Path: "/synced-menu",
		Type: 2,
	}})
	if err != nil {
		t.Fatalf("SyncMenus() error = %v", err)
	}
	if created != 1 {
		t.Fatalf("SyncMenus() created = %d, want 1", created)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestAssigningAPIsToMenuUsesAuthorizationVersionForAllUsers(t *testing.T) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "menu-api-user",
		Password:     "not-used",
		Email:        "menu-api-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 9,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create menu API user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create menu API user access version: %v", err)
	}
	path := "/menu-api"
	menu := model.Menu{
		Name:   "menu API",
		Path:   &path,
		Type:   2,
		Status: 1,
	}
	if err := db.Create(&menu).Error; err != nil {
		t.Fatalf("create menu: %v", err)
	}
	api := model.API{
		Name:           "menu API endpoint",
		Method:         "GET",
		Path:           "/api/menu-api",
		PermissionCode: "menu.api.get",
		Status:         1,
		NeedAuth:       1,
	}
	if err := db.Create(&api).Error; err != nil {
		t.Fatalf("create API: %v", err)
	}

	if err := AssignAPIsToMenu(menu.ID, dto.AssignAPIsToMenuReq{
		APIIDs: []uint{api.ID},
	}); err != nil {
		t.Fatalf("AssignAPIsToMenu() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestCreatingMenuButtonFromAPIUsesAuthorizationVersionForAllUsers(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "menu-button-user",
		Password:     "not-used",
		Email:        "menu-button-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 10,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create menu button user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create menu button user access version: %v", err)
	}
	parentPath := "/menu-button-parent"
	parent := model.Menu{
		Name:   "menu button parent",
		Path:   &parentPath,
		Type:   2,
		Status: 1,
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent menu: %v", err)
	}
	api := model.API{
		Name:           "menu button API",
		Method:         "POST",
		Path:           "/api/menu-button",
		PermissionCode: "menu.button.post",
		Status:         1,
		NeedAuth:       1,
	}
	if err := db.Create(&api).Error; err != nil {
		t.Fatalf("create API: %v", err)
	}

	if _, err := CreateMenuButtonFromAPI(api.ID, dto.GenerateMenuButtonFromAPIReq{
		ParentID: parent.ID,
		Name:     "generated button",
	}); err != nil {
		t.Fatalf("CreateMenuButtonFromAPI() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestUpdatingLinkedMenuPermissionUsesAuthorizationVersionForAllUsers(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "linked-menu-user",
		Password:     "not-used",
		Email:        "linked-menu-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 11,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create linked menu user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create linked menu user access version: %v", err)
	}
	path := "/linked-menu"
	menu := model.Menu{
		Name:           "linked menu",
		Path:           &path,
		PermissionCode: "linked.menu.before",
		Type:           2,
		Status:         1,
	}
	if err := db.Create(&menu).Error; err != nil {
		t.Fatalf("create linked menu: %v", err)
	}
	api := model.API{
		Name:           "linked menu API",
		Method:         "GET",
		Path:           "/api/linked-menu",
		PermissionCode: menu.PermissionCode,
		Status:         1,
		NeedAuth:       1,
	}
	if err := db.Create(&api).Error; err != nil {
		t.Fatalf("create linked API: %v", err)
	}
	if err := db.Create(&model.MenuAPI{
		MenuID: menu.ID,
		APIID:  api.ID,
	}).Error; err != nil {
		t.Fatalf("link menu API: %v", err)
	}

	permissionCode := "linked.menu.after"
	if _, err := UpdateAPI(api.ID, dto.UpdateAPIReq{
		PermissionCode: &permissionCode,
	}); err != nil {
		t.Fatalf("UpdateAPI() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		user.ID,
		user.TokenVersion+1,
	)
}

func TestUpdatingLinkedAPISucceedsWithoutLegacyMirrorWrite(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	user := exitAccessVersionTestUser{
		Username:     "linked-api-rollback-user",
		Password:     "not-used",
		Email:        "linked-api-rollback-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 6,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create linked API rollback user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: user.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create linked API rollback access version: %v", err)
	}
	path := "/linked-api-rollback"
	menu := model.Menu{
		Name:           "linked API rollback menu",
		Path:           &path,
		PermissionCode: "linked.api.before",
		Type:           2,
		Status:         1,
	}
	if err := db.Create(&menu).Error; err != nil {
		t.Fatalf("create linked API rollback menu: %v", err)
	}
	api := model.API{
		Name:           "linked API rollback metadata",
		Method:         "GET",
		Path:           "/api/linked-api-rollback",
		PermissionCode: menu.PermissionCode,
		Status:         1,
		NeedAuth:       1,
	}
	if err := db.Create(&api).Error; err != nil {
		t.Fatalf("create linked API rollback metadata: %v", err)
	}
	if err := db.Create(&model.MenuAPI{
		MenuID: menu.ID,
		APIID:  api.ID,
	}).Error; err != nil {
		t.Fatalf("link rollback menu and API: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_linked_api_legacy_mirror
		BEFORE UPDATE OF token_version ON users
		BEGIN
			SELECT RAISE(ABORT, 'forced linked API mirror failure');
		END
	`).Error; err != nil {
		t.Fatalf("create linked API mirror failure trigger: %v", err)
	}

	permissionCode := "linked.api.after"
	if _, err := UpdateAPI(api.ID, dto.UpdateAPIReq{
		PermissionCode: &permissionCode,
	}); err != nil {
		t.Fatalf("UpdateAPI() with legacy-write rejection error = %v", err)
	}

	var storedAPI model.API
	if err := db.First(&storedAPI, api.ID).Error; err != nil {
		t.Fatalf("reload API after exit-version update: %v", err)
	}
	if storedAPI.PermissionCode != permissionCode {
		t.Fatalf(
			"API permission code after exit-version update = %q, want %q",
			storedAPI.PermissionCode,
			permissionCode,
		)
	}
	var storedMenu model.Menu
	if err := db.First(&storedMenu, menu.ID).Error; err != nil {
		t.Fatalf("reload menu after exit-version update: %v", err)
	}
	if storedMenu.PermissionCode != permissionCode {
		t.Fatalf(
			"menu permission code after exit-version update = %q, want %q",
			storedMenu.PermissionCode,
			permissionCode,
		)
	}
	assertTokenVersionRegressionState(
		t,
		db,
		user.ID,
		1,
		user.TokenVersion,
	)
	assertAuthorizationVersion(t, db, user.ID, user.TokenVersion+1)
}

func TestOrganizationMembershipChangeUsesAuthorizationVersionForRemovedAndAddedUsers(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	allDataRole := model.Role{
		Name:      "organization administrator",
		Code:      "organization-administrator",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&allDataRole).Error; err != nil {
		t.Fatalf("create all-data role: %v", err)
	}
	operator := exitAccessVersionTestUser{
		Username: "organization-operator",
		Password: "not-used",
		Email:    "organization-operator@example.com",
		Role:     "user",
		Status:   1,
	}
	oldUser := exitAccessVersionTestUser{
		Username:     "old-organization-user",
		Password:     "not-used",
		Email:        "old-organization-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 3,
	}
	newUser := exitAccessVersionTestUser{
		Username:     "new-organization-user",
		Password:     "not-used",
		Email:        "new-organization-user@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 6,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create organization operator: %v", err)
	}
	if err := db.Create(&oldUser).Error; err != nil {
		t.Fatalf("create removed organization user: %v", err)
	}
	if err := db.Create(&newUser).Error; err != nil {
		t.Fatalf("create added organization user: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: operator.ID,
		RoleID: allDataRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign operator role: %v", err)
	}
	for _, user := range []exitAccessVersionTestUser{oldUser, newUser} {
		if err := db.Create(&model.UserAccessVersion{
			UserID:  user.ID,
			Version: user.TokenVersion,
		}).Error; err != nil {
			t.Fatalf("create %s access version: %v", user.Username, err)
		}
	}
	organization := model.Organization{
		Name:   "organization membership",
		Code:   "organization-membership",
		Status: 1,
	}
	if err := db.Create(&organization).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := db.Create(&model.UserOrganization{
		UserID:         oldUser.ID,
		OrganizationID: organization.ID,
	}).Error; err != nil {
		t.Fatalf("assign old organization user: %v", err)
	}

	if err := SetOrganizationUsers(
		operator.ID,
		organization.ID,
		[]uint{newUser.ID},
	); err != nil {
		t.Fatalf("SetOrganizationUsers() error = %v", err)
	}

	for _, user := range []exitAccessVersionTestUser{oldUser, newUser} {
		assertTokenVersionRegressionState(
			t,
			db,
			user.ID,
			1,
			user.TokenVersion,
		)
		assertAuthorizationVersion(
			t,
			db,
			user.ID,
			user.TokenVersion+1,
		)
	}
}

func TestDeletingOrganizationUsesAuthorizationVersionForFormerMembers(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)

	allDataRole := model.Role{
		Name:      "organization-delete-administrator",
		Code:      "organization-delete-administrator",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&allDataRole).Error; err != nil {
		t.Fatalf("create all-data role: %v", err)
	}
	operator := exitAccessVersionTestUser{
		Username: "organization-delete-operator",
		Password: "not-used",
		Email:    "organization-delete-operator@example.com",
		Role:     "user",
		Status:   1,
	}
	member := exitAccessVersionTestUser{
		Username:     "deleted-organization-member",
		Password:     "not-used",
		Email:        "deleted-organization-member@example.com",
		Role:         "user",
		Status:       1,
		TokenVersion: 12,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create organization operator: %v", err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create organization member: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: operator.ID,
		RoleID: allDataRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign operator role: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  member.ID,
		Version: member.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create member access version: %v", err)
	}
	organization := model.Organization{
		Name:   "organization to delete",
		Code:   "organization-to-delete",
		Status: 1,
	}
	if err := db.Create(&organization).Error; err != nil {
		t.Fatalf("create organization: %v", err)
	}
	if err := db.Create(&model.UserOrganization{
		UserID:         member.ID,
		OrganizationID: organization.ID,
	}).Error; err != nil {
		t.Fatalf("assign organization member: %v", err)
	}

	if err := DeleteOrganization(operator.ID, organization.ID); err != nil {
		t.Fatalf("DeleteOrganization() error = %v", err)
	}

	assertTokenVersionRegressionState(
		t,
		db,
		member.ID,
		1,
		member.TokenVersion,
	)
	assertAuthorizationVersion(
		t,
		db,
		member.ID,
		member.TokenVersion+1,
	)
}

func TestDeletingUserSoftDeletesAndIncrementsAccessVersionInOneTransaction(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: target.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}

	if err := DeleteUserByAdmin(operator.ID, target.ID); err != nil {
		t.Fatalf("DeleteUserByAdmin() error = %v", err)
	}

	var stored exitAccessVersionTestUser
	if err := db.Unscoped().First(&stored, target.ID).Error; err != nil {
		t.Fatalf("reload soft-deleted user: %v", err)
	}
	if !stored.DeletedAt.Valid {
		t.Fatal("target user was not soft deleted")
	}
	assertAuthorizationVersion(
		t,
		db,
		target.ID,
		target.TokenVersion+1,
	)
}

func TestRestoringSoftDeletedUserKeepsDeletionVersionAndRejectsPreDeleteTokens(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: target.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}
	accessToken, refreshToken := generateTokenVersionRegressionPair(t, target)

	if err := DeleteUserByAdmin(operator.ID, target.ID); err != nil {
		t.Fatalf("DeleteUserByAdmin() error = %v", err)
	}

	var accessVersionCount int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", target.ID).
		Count(&accessVersionCount).Error; err != nil {
		t.Fatalf("count access versions after soft delete: %v", err)
	}
	if accessVersionCount != 1 {
		t.Fatalf(
			"access-version rows after soft delete = %d, want retained 1",
			accessVersionCount,
		)
	}

	if err := db.Unscoped().
		Model(&exitAccessVersionTestUser{}).
		Where("id = ?", target.ID).
		Update("deleted_at", nil).Error; err != nil {
		t.Fatalf("restore soft-deleted target through database adapter: %v", err)
	}

	var restored exitAccessVersionTestUser
	if err := db.First(&restored, target.ID).Error; err != nil {
		t.Fatalf("reload restored target: %v", err)
	}
	if restored.DeletedAt.Valid {
		t.Fatal("target user remained soft deleted after restoration")
	}
	assertAuthorizationVersion(
		t,
		db,
		target.ID,
		target.TokenVersion+1,
	)
	assertTokenVersionRegressionPairRejected(t, accessToken, refreshToken)
}

func TestDisablingUserBeforeFirstLoginCreatesAndIncrementsAccessVersion(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	status, err := ToggleUserStatus(operator.ID, target.ID)
	if err != nil {
		t.Fatalf("ToggleUserStatus() error = %v", err)
	}
	if status != 0 {
		t.Fatalf("ToggleUserStatus() status = %d, want disabled 0", status)
	}

	assertTokenVersionRegressionState(t, db, target.ID, 0, 1)
	assertAuthorizationVersion(t, db, target.ID, 2)
}

func TestDeletingUserBeforeFirstLoginCreatesAndIncrementsAccessVersion(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := DeleteUserByAdmin(operator.ID, target.ID); err != nil {
		t.Fatalf("DeleteUserByAdmin() error = %v", err)
	}

	var deleted exitAccessVersionTestUser
	if err := db.Unscoped().First(&deleted, target.ID).Error; err != nil {
		t.Fatalf("reload soft-deleted pre-login user: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatal("pre-login user was not soft deleted")
	}
	assertAuthorizationVersion(t, db, target.ID, 2)
}

func TestKickingUserBeforeFirstLoginCreatesAndIncrementsAccessVersion(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := KickUserByAdmin(operator.ID, target.ID); err != nil {
		t.Fatalf("KickUserByAdmin() error = %v", err)
	}

	assertTokenVersionRegressionState(t, db, target.ID, 1, 1)
	assertAuthorizationVersion(t, db, target.ID, 2)
}

func TestAssigningAuthorizationBeforeFirstLoginCreatesAndIncrementsAccessVersion(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	_, target := createTokenVersionRegressionUsers(t, db)
	role := model.Role{
		Name:      "pre-login-authorization-role",
		Code:      "pre-login-authorization-role",
		Status:    1,
		DataScope: model.DataScopeSelf,
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create pre-login authorization role: %v", err)
	}

	if err := AssignUsersToRole(role.ID, []uint{target.ID}); err != nil {
		t.Fatalf("AssignUsersToRole() error = %v", err)
	}

	assertTokenVersionRegressionState(t, db, target.ID, 1, 1)
	assertAuthorizationVersion(t, db, target.ID, 2)
}

func TestTogglingUserStatusRollsBackWhenAccessVersionPrimaryWriteFails(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: target.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_status_access_version_primary_write
		BEFORE UPDATE OF version ON user_access_versions
		BEGIN
			SELECT RAISE(ABORT, 'forced access-version primary write failure');
		END
	`).Error; err != nil {
		t.Fatalf("create primary-write failure trigger: %v", err)
	}

	if _, err := ToggleUserStatus(operator.ID, target.ID); err == nil {
		t.Fatal("ToggleUserStatus() error = nil, want primary-write failure")
	}

	assertTokenVersionRegressionState(
		t,
		db,
		target.ID,
		target.Status,
		target.TokenVersion,
	)
	assertAuthorizationVersion(t, db, target.ID, target.TokenVersion)
}

func TestDeletingUserSucceedsWithoutLegacyMirrorWrite(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: target.TokenVersion,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_delete_access_version_mirror_write
		BEFORE UPDATE OF token_version ON users
		BEGIN
			SELECT RAISE(ABORT, 'forced access-version mirror write failure');
		END
	`).Error; err != nil {
		t.Fatalf("create mirror-write failure trigger: %v", err)
	}

	if err := DeleteUserByAdmin(operator.ID, target.ID); err != nil {
		t.Fatalf(
			"DeleteUserByAdmin() with legacy-write rejection error = %v",
			err,
		)
	}

	var stored exitAccessVersionTestUser
	if err := db.Unscoped().First(&stored, target.ID).Error; err != nil {
		t.Fatalf("reload target after exit-version delete: %v", err)
	}
	if !stored.DeletedAt.Valid {
		t.Fatal("target was not soft deleted by exit version")
	}
	assertAuthorizationVersion(t, db, target.ID, target.TokenVersion+1)
}

func TestDeletingUserBeforeFirstLoginRollsBackWhenAccessVersionCreateFails(
	t *testing.T,
) {
	db := openTokenVersionRegressionDB(t)
	installTokenVersionRegressionGlobals(t, db)
	operator, target := createTokenVersionRegressionUsers(t, db)
	if err := db.Exec(`
		CREATE TRIGGER reject_delete_access_version_create
		BEFORE INSERT ON user_access_versions
		BEGIN
			SELECT RAISE(ABORT, 'forced access-version create failure');
		END
	`).Error; err != nil {
		t.Fatalf("create access-version create failure trigger: %v", err)
	}

	if err := DeleteUserByAdmin(operator.ID, target.ID); err == nil {
		t.Fatal("DeleteUserByAdmin() error = nil, want primary create failure")
	}

	var stored exitAccessVersionTestUser
	if err := db.First(&stored, target.ID).Error; err != nil {
		t.Fatalf("reload target after failed pre-login delete: %v", err)
	}
	if stored.DeletedAt.Valid {
		t.Fatal("target remained soft deleted after primary-create rollback")
	}
	var accessVersionCount int64
	if err := db.Model(&model.UserAccessVersion{}).
		Where("user_id = ?", target.ID).
		Count(&accessVersionCount).Error; err != nil {
		t.Fatalf("count access versions after failed create: %v", err)
	}
	if accessVersionCount != 0 {
		t.Fatalf(
			"access-version rows after failed create = %d, want 0",
			accessVersionCount,
		)
	}
}
