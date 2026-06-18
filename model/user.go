package model

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Username string `gorm:"type:varchar(100);not null;uniqueIndex;comment:用户名"`
	Password string `gorm:"type:varchar(255);not null;comment:密码"`
	Nickname string `gorm:"type:varchar(100);comment:昵称"`
	Avatar   string `gorm:"type:varchar(255);comment:头像"`
	Role     string `gorm:"type:varchar(50);default:user;comment:角色"`
	Status   int    `gorm:"type:tinyint;default:1;comment:状态 1启用 0禁用"`
	Email    string `gorm:"type:varchar(100);not null;uniqueIndex;comment:邮箱"`
}
