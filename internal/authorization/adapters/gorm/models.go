package gormadapter

import (
	"time"

	"gorm.io/gorm"
)

type Role struct {
	gorm.Model
	Name        string `gorm:"type:varchar(50);not null;uniqueIndex;comment:角色名称"`
	Code        string `gorm:"type:varchar(50);not null;uniqueIndex;comment:角色编码"`
	Description string `gorm:"type:varchar(255);comment:角色描述"`
	Sort        int    `gorm:"type:int;default:0;comment:排序"`
	Status      int    `gorm:"type:tinyint;default:1;comment:状态 1启用 0禁用"`
	DataScope   string `gorm:"type:varchar(50);default:all;comment:数据范围 all/self/org/org_and_children/custom"`
}

type UserRole struct {
	UserID uint `gorm:"primaryKey;comment:用户ID"`
	RoleID uint `gorm:"primaryKey;comment:角色ID"`
}

type RoleDataScope struct {
	RoleID         uint `gorm:"primaryKey;comment:角色ID"`
	OrganizationID uint `gorm:"primaryKey;index;comment:组织ID"`
}

type Permission struct {
	gorm.Model
	Name  string `gorm:"type:varchar(100);not null;comment:权限名称"`
	Code  string `gorm:"type:varchar(100);not null;uniqueIndex;comment:权限码"`
	Group string `gorm:"type:varchar(50);comment:权限分组"`
	Sort  int    `gorm:"type:int;default:0;comment:排序"`
}

type RolePermission struct {
	RoleID       uint `gorm:"primaryKey;comment:角色ID"`
	PermissionID uint `gorm:"primaryKey;comment:权限ID"`
}

type PermissionGroup struct {
	gorm.Model
	Name string `gorm:"type:varchar(50);not null;uniqueIndex;comment:分组名称"`
	Sort int    `gorm:"type:int;default:0;comment:排序"`
}

type UserAccessVersion struct {
	UserID    uint      `gorm:"primaryKey;comment:用户ID"`
	Version   int       `gorm:"type:int;not null;default:1;check:version > 0;comment:授权版本"`
	CreatedAt time.Time `gorm:"comment:创建时间"`
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}
