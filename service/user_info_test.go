package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
)

func TestUserInfoFromModelUsesControlledAvatarURLForTrustedAvatar(t *testing.T) {
	user := model.User{
		Username:               "alice",
		Nickname:               "Alice",
		Avatar:                 "https://legacy.example/avatar.png",
		AvatarObjectName:       "avatars/42/00000000-0000-0000-0000-000000000001.jpg",
		AvatarContentType:      "image/jpeg",
		AvatarValidationStatus: model.FileValidationStatusValidated,
		Email:                  "alice@example.com",
		Role:                   "user",
		Status:                 1,
	}
	user.ID = 42

	info := UserInfoFromModel(user)

	if info.Avatar != "/api/avatars/42" {
		t.Fatalf("avatar = %q, want controlled user avatar URL", info.Avatar)
	}
	if info.ID != 42 ||
		info.Username != "alice" ||
		info.Nickname != "Alice" ||
		info.Email != "alice@example.com" ||
		info.Role != "user" ||
		info.Status != 1 {
		t.Fatalf("user info = %#v, want mapped non-sensitive fields", info)
	}
}

func TestUserInfoFromModelFallsBackForUntrustedAvatarMetadata(t *testing.T) {
	tests := []struct {
		name string
		user model.User
	}{
		{
			name: "legacy URL only",
			user: model.User{
				Avatar: "https://legacy.example/tracker.png",
			},
		},
		{
			name: "unvalidated object",
			user: model.User{
				AvatarObjectName:       "avatars/42/00000000-0000-0000-0000-000000000001.jpg",
				AvatarContentType:      "image/jpeg",
				AvatarValidationStatus: model.FileValidationStatusLegacyUnverified,
			},
		},
		{
			name: "object belongs to another user",
			user: model.User{
				AvatarObjectName:       "avatars/41/00000000-0000-0000-0000-000000000001.jpg",
				AvatarContentType:      "image/jpeg",
				AvatarValidationStatus: model.FileValidationStatusValidated,
			},
		},
		{
			name: "MIME does not match extension",
			user: model.User{
				AvatarObjectName:       "avatars/42/00000000-0000-0000-0000-000000000001.png",
				AvatarContentType:      "image/jpeg",
				AvatarValidationStatus: model.FileValidationStatusValidated,
			},
		},
		{
			name: "UUID is not canonical lowercase",
			user: model.User{
				AvatarObjectName:       "avatars/42/AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA.jpg",
				AvatarContentType:      "image/jpeg",
				AvatarValidationStatus: model.FileValidationStatusValidated,
			},
		},
		{
			name: "extension is not normalized output",
			user: model.User{
				AvatarObjectName:       "avatars/42/00000000-0000-0000-0000-000000000001.webp",
				AvatarContentType:      "image/webp",
				AvatarValidationStatus: model.FileValidationStatusValidated,
			},
		},
		{
			name: "user ID is not persisted",
			user: model.User{
				AvatarObjectName:       "avatars/0/00000000-0000-0000-0000-000000000001.jpg",
				AvatarContentType:      "image/jpeg",
				AvatarValidationStatus: model.FileValidationStatusValidated,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name != "user ID is not persisted" {
				tt.user.ID = 42
			}

			info := UserInfoFromModel(tt.user)
			if info.Avatar != "/api/avatars/default" {
				t.Fatalf("avatar = %q, want controlled default URL", info.Avatar)
			}
		})
	}
}

func TestGetUserInfoFallsBackToDefaultForLegacyAvatarURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}

	user := model.User{
		Username: "legacy",
		Password: "not-used",
		Avatar:   "https://legacy.example/tracker.png",
		Email:    "legacy@example.com",
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	info, err := GetUserInfo(user.ID)
	if err != nil {
		t.Fatalf("GetUserInfo() error = %v", err)
	}
	if info.Avatar != "/api/avatars/default" {
		t.Fatalf("avatar = %q, want system default without proxying legacy URL", info.Avatar)
	}
}

func TestGetRoleUsersUsesControlledAvatarURLs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Role{}, &model.UserRole{}); err != nil {
		t.Fatalf("migrate role users: %v", err)
	}

	role := model.Role{Name: "Auditor", Code: "auditor", Status: 1}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	user := model.User{
		Username: "legacy-role-user",
		Password: "not-used",
		Avatar:   "https://legacy.example/role-user.png",
		Email:    "legacy-role-user@example.com",
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserRole{UserID: user.ID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("assign role: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	users, err := GetRoleUsers(role.ID)
	if err != nil {
		t.Fatalf("GetRoleUsers() error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("role users = %d, want 1", len(users))
	}
	if users[0].Avatar != "/api/avatars/default" {
		t.Fatalf("avatar = %q, want controlled default URL", users[0].Avatar)
	}
}

func TestGetAllUsersUsesControlledAvatarURLs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.UserRole{},
		&model.UserOrganization{},
	); err != nil {
		t.Fatalf("migrate admin users: %v", err)
	}

	operator := model.User{
		Username: "legacy-operator",
		Password: "not-used",
		Avatar:   "https://legacy.example/operator.png",
		Email:    "legacy-operator@example.com",
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	users, total, err := GetAllUsers(operator.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetAllUsers() error = %v", err)
	}
	if total != 1 || len(users) != 1 {
		t.Fatalf("admin users = %d/%d, want 1/1", len(users), total)
	}
	if users[0].Avatar != "/api/avatars/default" {
		t.Fatalf("avatar = %q, want controlled default URL", users[0].Avatar)
	}
}

func TestUpdateUserByAdminUsesControlledAvatarURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
		&model.Role{},
		&model.UserRole{},
		&model.UserOrganization{},
	); err != nil {
		t.Fatalf("migrate admin update: %v", err)
	}

	operator := model.User{
		Username: "operator",
		Password: "not-used",
		Email:    "operator@example.com",
		Role:     "user",
		Status:   1,
	}
	target := model.User{
		Username: "legacy-target",
		Password: "not-used",
		Avatar:   "https://legacy.example/target.png",
		Email:    "legacy-target@example.com",
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  target.ID,
		Version: 1,
	}).Error; err != nil {
		t.Fatalf("create target access version: %v", err)
	}
	allDataRole := model.Role{
		Name:      "All data",
		Code:      "manager",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&allDataRole).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: operator.ID,
		RoleID: allDataRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign operator role: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	info, err := UpdateUserByAdmin(operator.ID, target.ID, dto.AdminUpdateUserReq{
		Nickname: "updated",
	})
	if err != nil {
		t.Fatalf("UpdateUserByAdmin() error = %v", err)
	}
	if info.Avatar != "/api/avatars/default" {
		t.Fatalf("avatar = %q, want controlled default URL", info.Avatar)
	}
}

func TestGetOrganizationUsersUsesControlledAvatarURLs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.UserRole{},
		&model.Organization{},
		&model.UserOrganization{},
	); err != nil {
		t.Fatalf("migrate organization users: %v", err)
	}

	operator := model.User{
		Username: "organization-operator",
		Password: "not-used",
		Email:    "organization-operator@example.com",
		Role:     "user",
		Status:   1,
	}
	member := model.User{
		Username: "legacy-member",
		Password: "not-used",
		Avatar:   "https://legacy.example/member.png",
		Email:    "legacy-member@example.com",
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
	allDataRole := model.Role{
		Name:      "Organization manager",
		Code:      "organization-manager",
		Status:    1,
		DataScope: model.DataScopeAll,
	}
	if err := db.Create(&allDataRole).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := db.Create(&model.UserRole{
		UserID: operator.ID,
		RoleID: allDataRole.ID,
	}).Error; err != nil {
		t.Fatalf("assign operator role: %v", err)
	}
	organization := model.Organization{
		Name:   "Security",
		Code:   "security",
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

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	users, err := GetOrganizationUsers(operator.ID, organization.ID)
	if err != nil {
		t.Fatalf("GetOrganizationUsers() error = %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("organization users = %d, want 1", len(users))
	}
	if users[0].Avatar != "/api/avatars/default" {
		t.Fatalf("avatar = %q, want controlled default URL", users[0].Avatar)
	}
}
