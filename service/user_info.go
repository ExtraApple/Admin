package service

import (
	"strconv"

	"admin/dto"
	"admin/model"
)

const defaultAvatarURL = "/api/avatars/default"

// UserInfoFromModel converts a user record to the public, password-free DTO.
// Avatar storage identifiers and legacy URLs are never exposed.
func UserInfoFromModel(user model.User) dto.UserInfo {
	avatarURL := defaultAvatarURL
	if _, ok := validAvatarObjectName(&user, user.ID); ok {
		avatarURL = "/api/avatars/" + strconv.FormatUint(uint64(user.ID), 10)
	}

	return dto.UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Avatar:   avatarURL,
		Email:    user.Email,
		Role:     user.Role,
		Status:   user.Status,
	}
}
