package model

import "gorm.io/gorm"

type Organization struct {
	gorm.Model
	ParentID uint   `gorm:"index;default:0;comment:父组织ID，0表示根组织"`
	Name     string `gorm:"type:varchar(100);not null;comment:组织名称"`
	Code     string `gorm:"type:varchar(100);not null;uniqueIndex;comment:组织编码"`
	Remark   string `gorm:"type:varchar(255);comment:备注"`
	Sort     int    `gorm:"type:int;default:0;comment:排序"`
	Status   int    `gorm:"type:tinyint;default:1;comment:状态 1启用 0禁用"`
}

type UserOrganization struct {
	UserID         uint `gorm:"primaryKey;comment:用户ID"`
	OrganizationID uint `gorm:"primaryKey;index;comment:组织ID"`
}
