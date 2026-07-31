package router

import (
	"time"

	"gorm.io/gorm"
)

// exitAccessVersionTestUser mirrors the exit-version users schema while
// keeping the expected authorization version as test-only, non-persistent data.
type exitAccessVersionTestUser struct {
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
	TokenVersion           int        `gorm:"-"`
	Email                  string     `gorm:"type:varchar(100);not null;uniqueIndex;comment:邮箱"`
}

func (exitAccessVersionTestUser) TableName() string {
	return "users"
}
