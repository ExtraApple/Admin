package model

import "gorm.io/gorm"

type Menu struct {
	gorm.Model
	ParentID       uint    `gorm:"index;comment:父级菜单ID"`
	Name           string  `gorm:"type:varchar(100);not null;comment:菜单名称"`
	Path           *string `gorm:"type:varchar(255);uniqueIndex;comment:前端路由路径"`
	Component      string  `gorm:"type:varchar(255);comment:前端组件路径"`
	Icon           string  `gorm:"type:varchar(100);comment:菜单图标"`
	PermissionCode string  `gorm:"type:varchar(100);index;comment:权限码"`
	Sort           int     `gorm:"type:int;default:0;comment:排序"`
	Type           int     `gorm:"type:tinyint;default:1;comment:类型 1目录 2菜单 3按钮"`
	Status         int     `gorm:"type:tinyint;default:1;comment:状态 1启用 0禁用"`
}

type RoleMenu struct {
	RoleID uint `gorm:"primaryKey;comment:角色ID"`
	MenuID uint `gorm:"primaryKey;comment:菜单ID"`
}
