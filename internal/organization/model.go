package organization

import "gorm.io/gorm"

type Unit struct {
	gorm.Model
	ParentID uint   `gorm:"index;default:0;comment:父组织ID，0表示根组织"`
	Name     string `gorm:"type:varchar(100);not null;comment:组织名称"`
	Code     string `gorm:"type:varchar(100);not null;uniqueIndex;comment:组织编码"`
	Remark   string `gorm:"type:varchar(255);comment:备注"`
	Sort     int    `gorm:"type:int;default:0;comment:排序"`
	Status   int    `gorm:"type:tinyint;not null;comment:状态 1启用 0禁用"`
}

func (Unit) TableName() string { return "organizations" }

type Membership struct {
	UserID         uint `gorm:"primaryKey;comment:用户ID"`
	OrganizationID uint `gorm:"primaryKey;index;comment:组织ID"`
}

func (Membership) TableName() string { return "user_organizations" }

func Models() []any { return []any{Unit{}, Membership{}} }
