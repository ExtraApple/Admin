package identity

import (
	"time"

	"gorm.io/gorm"
)

// User is the Identity-owned persistence record for a user.
type User struct {
	gorm.Model
	Username               string     `gorm:"type:varchar(100);not null;uniqueIndex;comment:用户名"`
	Password               string     `gorm:"type:varchar(255);not null;comment:密码"`
	Nickname               string     `gorm:"type:varchar(100);comment:昵称"`
	Avatar                 string     `gorm:"type:varchar(255);default:'http://127.0.0.1:9001/browser/image/normal.png';comment:旧头像兼容字段"`
	AvatarObjectName       string     `gorm:"type:varchar(500);comment:可信头像对象名"`
	AvatarContentType      string     `gorm:"type:varchar(100);comment:头像规范 MIME 类型"`
	AvatarContentSHA256    string     `gorm:"type:varchar(64);comment:服务端计算的头像内容 SHA-256 摘要"`
	AvatarValidationStatus string     `gorm:"type:varchar(32);index;comment:头像验证状态"`
	AvatarValidatedAt      *time.Time `gorm:"comment:头像验证完成时间"`
	Role                   string     `gorm:"type:varchar(50);default:user;comment:角色"`
	Status                 int        `gorm:"type:tinyint;default:1;comment:状态 1启用 0禁用"`
	Email                  string     `gorm:"type:varchar(100);not null;uniqueIndex;comment:当前邮箱"`
	PendingEmail           string     `gorm:"type:varchar(100);comment:待验证邮箱"`
	EmailVerifiedAt        *time.Time `gorm:"comment:邮箱验证完成时间"`
}

// EmailVerificationCredential is a one-time, hashed email verification token.
type EmailVerificationCredential struct {
	gorm.Model
	UserID    uint       `gorm:"not null;index;comment:用户 ID"`
	Email     string     `gorm:"type:varchar(100);not null;index;comment:验证目标邮箱"`
	TokenHash string     `gorm:"type:char(64);not null;uniqueIndex;comment:SHA-256 token 摘要"`
	ExpiresAt time.Time  `gorm:"not null;index;comment:凭据过期时间"`
	UsedAt    *time.Time `gorm:"index;comment:凭据消费时间"`
}

// Models returns the persistence models owned by Identity.
func Models() []any { return []any{User{}, EmailVerificationCredential{}} }
