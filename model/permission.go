package model

import "gorm.io/gorm"

// Permission 权限表 — 存储权限码（如 "user:delete"）
type Permission struct {
	gorm.Model
	Name  string `gorm:"type:varchar(100);not null;comment:权限名称"`
	Code  string `gorm:"type:varchar(100);not null;uniqueIndex;comment:权限码"`
	Group string `gorm:"type:varchar(50);comment:权限分组"`
	Sort  int    `gorm:"type:int;default:0;comment:排序"`
}

// RolePermission 角色-权限关联表
type RolePermission struct {
	RoleID       uint `gorm:"primaryKey;comment:角色ID"`
	PermissionID uint `gorm:"primaryKey;comment:权限ID"`
}

// PermissionGroup 权限分组表
type PermissionGroup struct {
	gorm.Model
	Name string `gorm:"type:varchar(50);not null;uniqueIndex;comment:分组名称"`
	Sort int    `gorm:"type:int;default:0;comment:排序"`
}
