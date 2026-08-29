package identity

import (
	"encoding/json"
	"strconv"

	"admin/internal/identity/domain"
)

// RegisterRequest is the public registration request contract.
type RegisterRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=100"`
	Password    string `json:"password" binding:"required,min=6,max=255"`
	Email       string `json:"email" binding:"required,email"`
	Nickname    string `json:"nickname"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required,len=6"`
}

// LoginRequest is the public login request contract.
type LoginRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required,len=6"`
}

// RefreshTokenRequest is the public refresh request contract.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type UserInfo struct {
	ID            uint   `json:"id"`
	Username      string `json:"username"`
	Nickname      string `json:"nickname"`
	Avatar        string `json:"avatar"`
	Email         string `json:"email"`
	PendingEmail  string `json:"pending_email"`
	EmailVerified bool   `json:"email_verified"`
	Role          string `json:"role"`
	Status        int    `json:"status"`
}

type LoginResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserInfo `json:"user"`
}

type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
type UpdateSelfRequest struct {
	Nickname        string `json:"nickname" binding:"max=100"`
	Email           string `json:"email" binding:"omitempty,email"`
	CurrentPassword string `json:"current_password"`

	avatarPresent bool
}


// UpdateSelfRequestSchema describes the conditional email re-authentication
// contract without changing the custom wire decoder used by the handler.
type UpdateSelfRequestSchema struct {
	Nickname        string `json:"nickname"`
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password" binding:"required"`
}
func (request *UpdateSelfRequest) UnmarshalJSON(data []byte) error {
	var wire struct {
		Nickname        string          `json:"nickname"`
		Avatar          json.RawMessage `json:"avatar"`
		Email           string          `json:"email"`
		CurrentPassword string          `json:"current_password"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	request.Nickname = wire.Nickname
	request.Email = wire.Email
	request.CurrentPassword = wire.CurrentPassword
	request.avatarPresent = wire.Avatar != nil
	return nil
}

func (request UpdateSelfRequest) HasAvatarField() bool { return request.avatarPresent }

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=255"`

	ConfirmPassword string `json:"confirm_password" binding:"required,min=6,max=255"`
}

type EmailVerificationRequest struct {
	Token string `json:"token" binding:"required"`
}

type UserListResponse struct {
	List  []UserInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

type AdminUpdateUserRequest struct {
	Nickname string `json:"nickname" binding:"max=100"`
	Email    string `json:"email" binding:"email"`
	Role     string `json:"role"`
	Status   *int   `json:"status"`
}

type InitialContextResponse struct {
	User        UserInfo     `json:"user"`
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"permissions"`
	Menus       []MenuDetail `json:"menus"`
}

type MenuDetail struct {
	ID             uint         `json:"id"`
	ParentID       uint         `json:"parent_id"`
	Name           string       `json:"name"`
	Path           string       `json:"path"`
	Component      string       `json:"component"`
	Icon           string       `json:"icon"`
	PermissionCode string       `json:"permission_code"`
	Sort           int          `json:"sort"`
	Type           int          `json:"type"`
	Status         int          `json:"status"`
	Children       []MenuDetail `json:"children,omitempty"`
}

func UserInfoFromDomain(user domain.User) UserInfo {
	avatar := "/api/avatars/default"
	if _, trusted := domain.TrustedAvatarObjectName(user, user.ID); trusted {
		avatar = "/api/avatars/" + strconv.FormatUint(uint64(user.ID), 10)
	}
	return UserInfo{ID: user.ID, Username: user.Username, Nickname: user.Nickname, Avatar: avatar, Email: user.Email, PendingEmail: user.PendingEmail, EmailVerified: user.EmailVerifiedAt != nil, Role: user.Role, Status: user.Status}
}

func MenuDetailsFromDomain(menus []domain.Menu) []MenuDetail {
	result := make([]MenuDetail, len(menus))
	for index, menu := range menus {
		result[index] = MenuDetail{ID: menu.ID, ParentID: menu.ParentID, Name: menu.Name, Path: menu.Path, Component: menu.Component, Icon: menu.Icon, PermissionCode: menu.PermissionCode, Sort: menu.Sort, Type: menu.Type, Status: menu.Status, Children: MenuDetailsFromDomain(menu.Children)}
	}
	return result
}
