package model

import "gorm.io/gorm"

type Role struct {
	gorm.Model
	Name        string `gorm:"type:varchar(50);not null;uniqueIndex;comment:角色名称"`
	Code        string `gorm:"type:varchar(50);not null;uniqueIndex;comment:角色编码"`
	Description string `gorm:"type:varchar(255);comment:角色描述"`
	Sort        int    `gorm:"type:int;default:0;comment:排序"`
	Status      int    `gorm:"type:tinyint;default:1;comment:状态 1启用 0禁用"`
}

// UserRole 用户-角色关联表
type UserRole struct {
	UserID uint `gorm:"primaryKey;comment:用户ID"`
	RoleID uint `gorm:"primaryKey;comment:角色ID"`
}
