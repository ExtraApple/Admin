package service

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"admin/dto"
	"admin/global"
	"admin/model"
	"admin/service/uploadsecurity"
)

func TestUpdateSelfRejectsAvatarFieldBeforePersistingOtherChanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}

	user := model.User{
		Username: "alice",
		Password: "not-used",
		Nickname: "before",
		Email:    "alice@example.com",
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

	var req dto.UpdateSelfReq
	if err := json.Unmarshal(
		[]byte(`{"nickname":"after","avatar":""}`),
		&req,
	); err != nil {
		t.Fatalf("decode update request: %v", err)
	}

	_, err = UpdateSelf(user.ID, req)
	if err == nil || err.Error() != "头像只能通过专用接口修改" {
		t.Fatalf("UpdateSelf() error = %v, want explicit avatar-field rejection", err)
	}

	var stored model.User
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if stored.Nickname != "before" {
		t.Fatalf("nickname = %q, want unchanged because whole request was rejected", stored.Nickname)
	}
	if stored.Avatar != user.Avatar {
		t.Fatalf("legacy avatar = %q, want unchanged %q", stored.Avatar, user.Avatar)
	}
}

func TestUpdateSelfRejectsNullAvatarField(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}

	user := model.User{
		Username: "bob",
		Password: "not-used",
		Nickname: "before",
		Email:    "bob@example.com",
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

	var req dto.UpdateSelfReq
	if err := json.Unmarshal(
		[]byte(`{"nickname":"after","avatar":null}`),
		&req,
	); err != nil {
		t.Fatalf("decode update request: %v", err)
	}

	_, err = UpdateSelf(user.ID, req)
	if err == nil || err.Error() != "头像只能通过专用接口修改" {
		t.Fatalf("UpdateSelf() error = %v, want null avatar-field rejection", err)
	}

	var stored model.User
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if stored.Nickname != "before" {
		t.Fatalf("nickname = %q, want unchanged because whole request was rejected", stored.Nickname)
	}
}

func TestUpdateSelfClassifiesEmptyProfileRequest(t *testing.T) {
	_, err := UpdateSelf(42, dto.UpdateSelfReq{})
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeRequestInvalid {
		t.Fatalf("UpdateSelf() code = %q, classified=%v, want %q; error=%v",
			code, ok, uploadsecurity.CodeRequestInvalid, err)
	}
}

func TestUpdateSelfUpdatesOnlyNicknameAndEmail(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}

	validatedAt := time.Date(2026, 7, 15, 9, 30, 0, 0, time.UTC)
	user := model.User{
		Username:               "carol",
		Password:               "password-hash-must-not-change",
		Nickname:               "before",
		Email:                  "carol@example.com",
		Avatar:                 "https://legacy.example.invalid/carol.png",
		AvatarObjectName:       "avatars/1/11111111-1111-1111-1111-111111111111.png",
		AvatarContentType:      "image/png",
		AvatarValidationStatus: model.FileValidationStatusValidated,
		AvatarValidatedAt:      &validatedAt,
		Role:                   "user",
		Status:                 1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	info, err := UpdateSelf(user.ID, dto.UpdateSelfReq{
		Nickname: "after",
		Email:    "carol.new@example.com",
	})
	if err != nil {
		t.Fatalf("UpdateSelf() error = %v", err)
	}
	if info == nil ||
		info.Nickname != "after" ||
		info.Email != "carol.new@example.com" ||
		info.Avatar != "/api/avatars/"+strconv.FormatUint(uint64(user.ID), 10) {
		t.Fatalf("UpdateSelf() info = %#v, want updated allowed fields and controlled avatar URL", info)
	}

	var stored model.User
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if stored.Nickname != "after" || stored.Email != "carol.new@example.com" {
		t.Fatalf("stored profile = nickname %q email %q, want updated values",
			stored.Nickname, stored.Email)
	}
	if stored.Password != user.Password ||
		stored.Username != user.Username ||
		stored.Role != user.Role ||
		stored.Status != user.Status ||
		stored.Avatar != user.Avatar ||
		stored.AvatarObjectName != user.AvatarObjectName ||
		stored.AvatarContentType != user.AvatarContentType ||
		stored.AvatarValidationStatus != user.AvatarValidationStatus ||
		stored.AvatarValidatedAt == nil ||
		!stored.AvatarValidatedAt.Equal(validatedAt) {
		t.Fatalf("UpdateSelf() changed protected fields: %#v", stored)
	}
}

func TestUpdateSelfRejectsEmailUsedByAnotherUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}

	users := []model.User{
		{
			Username: "dave",
			Password: "not-used",
			Nickname: "before",
			Email:    "dave@example.com",
			Role:     "user",
			Status:   1,
		},
		{
			Username: "erin",
			Password: "not-used",
			Nickname: "erin",
			Email:    "erin@example.com",
			Role:     "user",
			Status:   1,
		},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatalf("create user %q: %v", users[i].Username, err)
		}
	}

	previousDB := global.DB
	global.DB = db
	t.Cleanup(func() {
		global.DB = previousDB
	})

	_, err = UpdateSelf(users[0].ID, dto.UpdateSelfReq{
		Nickname: "must-not-change",
		Email:    users[1].Email,
	})
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeRequestInvalid {
		t.Fatalf("UpdateSelf() code = %q, classified=%v, want %q; error=%v",
			code, ok, uploadsecurity.CodeRequestInvalid, err)
	}

	var stored model.User
	if err := db.First(&stored, users[0].ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if stored.Nickname != users[0].Nickname || stored.Email != users[0].Email {
		t.Fatalf("profile changed after duplicate email rejection: %#v", stored)
	}
}
