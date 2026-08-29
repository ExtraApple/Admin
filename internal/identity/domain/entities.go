package domain

import "time"

type User struct {
	ID                     uint
	Username               string
	Password               string
	Nickname               string
	Avatar                 string
	AvatarObjectName       string
	AvatarContentType      string
	AvatarContentSHA256    string
	AvatarValidationStatus string
	AvatarValidatedAt      *time.Time
	Role                   string
	Status                 int
	Email                  string
	PendingEmail           string
	EmailVerifiedAt        *time.Time
}

type EmailVerificationCredential struct {
	ID        uint
	UserID    uint
	Email     string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (user User) Enabled() bool { return user.Status == 1 }

type AccessSnapshot struct {
	Roles       []string
	Permissions []string
	Version     int
}

type Menu struct {
	ID             uint
	ParentID       uint
	Name           string
	Path           string
	Component      string
	Icon           string
	PermissionCode string
	Sort           int
	Type           int
	Status         int
	Children       []Menu
}

type UserScope struct {
	All     bool
	UserIDs []uint
}
